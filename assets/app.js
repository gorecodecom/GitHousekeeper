      // ===========================================
      // Error Handling & User Feedback System
      // ===========================================

      let isProcessRunning = false;
      let serverHealthy = true;
      let healthCheckInterval = null;

      // ===========================================
      // Accessibility (a11y) Helpers
      // ===========================================

      // Keyboard Navigation for Sidebar
      function handleNavKeydown(event, tabName) {
        if (event.key === 'Enter' || event.key === ' ') {
          event.preventDefault();
          showTab(tabName);
        }
        // Arrow key navigation
        if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
          event.preventDefault();
          const navItems = document.querySelectorAll('.nav-item[role="menuitem"]');
          const currentIndex = Array.from(navItems).indexOf(event.target);
          let nextIndex;
          if (event.key === 'ArrowDown') {
            nextIndex = (currentIndex + 1) % navItems.length;
          } else {
            nextIndex = (currentIndex - 1 + navItems.length) % navItems.length;
          }
          navItems[nextIndex].focus();
        }
      }

      // Toast Notification System
      function showToast(title, message, type = 'info', duration = 5000) {
        const container = document.getElementById('toast-container');
        const toast = document.createElement('div');
        toast.className = `toast ${type}`;
        toast.innerHTML = `
          <div class="toast-content">
            <div class="toast-title">${title}</div>
            <div class="toast-message">${message}</div>
          </div>
          <button class="toast-close" onclick="this.parentElement.remove()">×</button>
        `;
        container.appendChild(toast);

        // Auto-remove after duration
        if (duration > 0) {
          setTimeout(() => {
            toast.classList.add('fade-out');
            setTimeout(() => toast.remove(), 300);
          }, duration);
        }

        return toast;
      }

      // Connection Status Banner
      function showConnectionBanner(type, icon, text) {
        const banner = document.getElementById('connection-banner');
        const iconEl = document.getElementById('connection-icon');
        const textEl = document.getElementById('connection-text');

        banner.className = `visible ${type}`;
        iconEl.textContent = icon;
        textEl.textContent = text;
      }

      function hideConnectionBanner() {
        const banner = document.getElementById('connection-banner');
        banner.classList.remove('visible');
      }

      // Online/Offline Detection
      window.addEventListener('offline', () => {
        showConnectionBanner('offline', '🔴', 'Keine Internetverbindung - Einige Funktionen sind möglicherweise eingeschränkt');
        showToast('Offline', 'Sie haben keine Internetverbindung.', 'warning');
      });

      window.addEventListener('online', () => {
        showConnectionBanner('reconnected', '🟢', 'Verbindung wiederhergestellt');
        showToast('Online', 'Internetverbindung wiederhergestellt.', 'success', 3000);
        setTimeout(hideConnectionBanner, 3000);
      });

      // Server Health Check
      async function checkServerHealth() {
        try {
          const controller = new AbortController();
          const timeoutId = setTimeout(() => controller.abort(), 5000);

          const res = await fetch('/api/health', {
            method: 'HEAD',
            signal: controller.signal
          });
          clearTimeout(timeoutId);

          if (!serverHealthy) {
            // Server was down, now back up
            serverHealthy = true;
            showConnectionBanner('reconnected', '🟢', 'Server-Verbindung wiederhergestellt');
            showToast('Server verbunden', 'Die Verbindung zum GitHousekeeper Server wurde wiederhergestellt.', 'success', 3000);
            setTimeout(hideConnectionBanner, 3000);
          }
        } catch (e) {
          if (serverHealthy) {
            serverHealthy = false;
            showConnectionBanner('server-error', '⚠️', 'Server nicht erreichbar - Bitte starten Sie GitHousekeeper neu');
            showToast('Server nicht erreichbar', 'Der GitHousekeeper Server antwortet nicht. Bitte überprüfen Sie, ob die Anwendung noch läuft.', 'error', 0);
          }
        }
      }

      // Start health check interval (every 30 seconds)
      function startHealthCheck() {
        if (healthCheckInterval) clearInterval(healthCheckInterval);
        healthCheckInterval = setInterval(checkServerHealth, 30000);
        // Initial check after 2 seconds
        setTimeout(checkServerHealth, 2000);
      }

      // Warning when closing with running process
      window.addEventListener('beforeunload', (event) => {
        if (isProcessRunning) {
          event.preventDefault();
          event.returnValue = 'Ein Prozess läuft noch. Möchten Sie die Seite wirklich verlassen?';
          return event.returnValue;
        }
      });

      // Enhanced fetch wrapper with error handling
      async function fetchWithErrorHandling(url, options = {}) {
        try {
          const response = await fetch(url, options);
          if (!response.ok) {
            throw new Error(`Server error: ${response.status}`);
          }
          serverHealthy = true;
          return response;
        } catch (error) {
          if (error.name === 'TypeError' || error.message.includes('fetch')) {
            serverHealthy = false;
            showConnectionBanner('server-error', '⚠️', 'Server nicht erreichbar');
            showToast('Verbindungsfehler', 'Der Server ist nicht erreichbar. Bitte überprüfen Sie, ob GitHousekeeper noch läuft.', 'error', 0);
          }
          throw error;
        }
      }

      // Initialize on page load
      document.addEventListener('DOMContentLoaded', () => {
        startHealthCheck();
      });

      // ===========================================
      // Main Application State
      // ===========================================

      let currentStats = {
        totalRepos: 0,
        repoDetails: [],
        springVersions: {},
        topDependencies: {},
        totalTodos: 0,
        totalHealth: 0
      };
      let lastLoadedPath = "";

      function showTab(tabId) {
        // Update Sidebar
        document
          .querySelectorAll(".nav-item")
          .forEach((el) => {
            el.classList.remove("active");
            el.removeAttribute("aria-current");
          });
        const activeNavItem = document.querySelector(`.nav-item[onclick="showTab('${tabId}')"]`);
        activeNavItem.classList.add("active");
        activeNavItem.setAttribute("aria-current", "page");

        // Update Content
        document
          .querySelectorAll(".tab-content")
          .forEach((el) => el.classList.remove("active"));
        document.getElementById(`tab-${tabId}`).classList.add("active");

        if (tabId === "frameworks") {
          loadSpringVersions();
          checkOpenRewriteVersions();
        }

        if (tabId === "dashboard") {
            // Priority 1: Current input value
            const currentPath = document.getElementById("rootPath").value;

            if (currentPath) {
                // Only reload if path changed
                if (currentPath !== lastLoadedPath) {
                    loadDashboardStats(currentPath);
                }
            } else {
                // Priority 2: Saved settings
                const saved = localStorage.getItem("gitHousekeeper_settings");
                if (saved) {
                    const s = JSON.parse(saved);
                    if (s.rootPath && s.rootPath !== lastLoadedPath) {
                        loadDashboardStats(s.rootPath);
                    }
                }
            }
        }
      }

      function toggleBranchInput() {
        const isCustom = document.getElementById("branch_custom").checked;
        const input = document.getElementById("customBranchName");
        input.disabled = !isCustom;
        if (isCustom) input.focus();
      }

      function addRow(containerId) {
        const container = document.getElementById(containerId);
        const div = document.createElement("div");
        div.className = "replacement-row";
        div.innerHTML = `
            <button class="btn-remove" onclick="removeRow(this)" title="Remove Row">-</button>
            <textarea placeholder="Search Text" class="replacement-search" oninput="autoResize(this)"></textarea>
            <textarea placeholder="Replacement" class="replacement-replace" oninput="autoResize(this)"></textarea>
        `;
        container.appendChild(div);
      }

      function removeRow(btn) {
        const row = btn.parentElement;
        // Optional: Prevent removing the last row if desired, but user asked to remove rows.
        // If we want to keep at least one, we can check row.parentElement.children.length
        row.remove();
      }

      function autoResize(el) {
        el.style.height = 'auto';
        el.style.height = el.scrollHeight + 'px';
      }

      function printSection(section) {
        document.body.classList.add("print-" + section);
        window.print();
        document.body.classList.remove("print-" + section);
      }

      function resetSettings() {
        if (
          !confirm(
            "Reset all settings? This will clear all saved paths, configurations and return to defaults."
          )
        ) {
          return;
        }

        // Clear localStorage
        localStorage.removeItem("gitHousekeeper_settings");

        // Reset all form fields
        document.getElementById("rootPath").value = "";
        document.getElementById("parentVersion").value = "";
        document.getElementById("versionBumpStrategy").value = "patch";
        document.getElementById("runCleanInstall").checked = false;
        document.getElementById("customBranchName").value = "";

        // Reset branch strategy to default (No Branch)
        document.getElementById("branch_none").checked = true;
        toggleBranchInput();

        // Reset folder list
        document.getElementById("folder-list-container").innerHTML =
          '<div class="hint">Select a root path to see folders.</div>';

        // Reset Replacements to single empty row
        const replacementsList = document.getElementById("replacements-list");
        if (replacementsList) {
          replacementsList.innerHTML = `
            <div class="replacement-row">
              <button class="btn-remove" onclick="removeRow(this)" title="Remove Row">-</button>
              <textarea placeholder="Search Text" class="replacement-search" oninput="autoResize(this)"></textarea>
              <textarea placeholder="Replacement" class="replacement-replace" oninput="autoResize(this)"></textarea>
            </div>
          `;
        }

        // Reset replacement scope to default
        const scopeAll = document.querySelector('input[name="replacementScope"][value="all"]');
        if (scopeAll) scopeAll.checked = true;

        // Clear report logs
        document.getElementById("report-log").innerHTML =
          '<div class="log-info">Ready to start...</div>';
        document.getElementById("deprecation-log").innerHTML =
          '<div class="log-info">Waiting for results...</div>';
      }

      async function runHousekeeper() {
        showTab("report");
        const log = document.getElementById("report-log");
        const deprecationLog = document.getElementById("deprecation-log");
        const loading = document.getElementById("loading");

        log.innerHTML = "";
        deprecationLog.innerHTML = "";
        loading.classList.remove("hidden");
        isProcessRunning = true; // Mark process as running;

        // Determine Target Branch
        let targetBranch = "";
        if (document.getElementById("branch_housekeeping").checked) {
          targetBranch = "housekeeping";
        } else if (document.getElementById("branch_custom").checked) {
          targetBranch = document
            .getElementById("customBranchName")
            .value.trim();
          if (!targetBranch) {
            alert("Please enter a branch name.");
            loading.classList.add("hidden");
            return;
          }
        } else {
          // None selected -> Master
          targetBranch = "";
        }

        // Collect Data
        // Calculate Excluded Folders from Checkboxes
        const excluded = ["node_modules", "target", "dist", ".idea", ".vscode"]; // Standard defaults
        document
          .querySelectorAll('#folder-list-container input[type="checkbox"]')
          .forEach((cb) => {
            if (!cb.checked) {
              excluded.push(cb.value);
            }
          });

        const data = {
          rootPath: document.getElementById("rootPath").value,
          excluded: excluded,
          parentVersion: document.getElementById("parentVersion").value,
          versionBumpStrategy: document.getElementById("versionBumpStrategy")
            .value,
          runCleanInstall: document.getElementById("runCleanInstall").checked,
          targetBranch: targetBranch,
          replacements: [],
          replacementScope: document.querySelector('input[name="replacementScope"]:checked')?.value || "all",
        };

        if (!data.rootPath) {
          alert("Please specify a project path.");
          loading.classList.add("hidden");
          showTab("settings");
          return;
        }

        // Save settings to localStorage
        localStorage.setItem(
          "gitHousekeeper_settings",
          JSON.stringify({
            rootPath: data.rootPath,
            // excluded: document.getElementById('excluded').value, // No longer saving excluded list as text
            parentVersion: data.parentVersion,
            versionBumpStrategy: data.versionBumpStrategy,
            runCleanInstall: data.runCleanInstall,
            branchStrategy: document.querySelector(
              'input[name="branchStrategy"]:checked'
            ).value,
            customBranchName: document.getElementById("customBranchName").value,
            replacementScope: data.replacementScope,
          })
        );

        // Collect Replacements
        document
          .querySelectorAll("#replacements-list .replacement-row")
          .forEach((row) => {
            const search = row.querySelector(".replacement-search").value;
            const replace = row.querySelector(".replacement-replace").value;
            if (search) {
              data.replacements.push({ Search: search, Replace: replace });
            }
          });

        try {
          const response = await fetch("/api/run", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(data),
          });

          const reader = response.body.getReader();
          const decoder = new TextDecoder("utf-8");
          let isDeprecation = false;

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            const chunk = decoder.decode(value, { stream: true });
            const lines = chunk.split("\n");

            for (let line of lines) {
              if (!line.trim()) continue;

              if (line.startsWith("DEPRECATION_START:")) {
                isDeprecation = true;
                const repoName = line.split(":")[1];
                const header = document.createElement("div");
                header.className = "log-repo";
                header.textContent = repoName;
                deprecationLog.appendChild(header);
                continue;
              }
              if (line.startsWith("DEPRECATION_END")) {
                isDeprecation = false;
                continue;
              }

              if (isDeprecation) {
                const div = document.createElement("div");
                div.className = "log-warning";
                div.textContent = line;
                deprecationLog.appendChild(div);
                deprecationLog.scrollTop = deprecationLog.scrollHeight;
                continue;
              }

              const div = document.createElement("div");
              if (line.startsWith("REPO:")) {
                div.className = "log-repo";
                div.textContent = line.substring(5);
              } else if (line.includes("[ERROR]") || line.includes("✗")) {
                div.className = "log-error";
                div.textContent = line;
              } else if (
                line.includes("[WARNING]") ||
                line.toLowerCase().includes("warning") ||
                line.toLowerCase().includes("deprecated")
              ) {
                div.className = "log-warning";
                div.textContent = line;
              } else if (line.includes("✓") || line.includes("success")) {
                div.className = "log-success";
                div.textContent = line;
              } else {
                div.className = "log-info";
                div.textContent = line;
              }
              log.appendChild(div);
              log.scrollTop = log.scrollHeight;
            }
          }
        } catch (e) {
          log.innerHTML += `<div class="log-error">Error: ${e.message}</div>`;
          showToast('Fehler', `Ein Fehler ist aufgetreten: ${e.message}`, 'error');
        } finally {
          loading.classList.add("hidden");
          isProcessRunning = false; // Mark process as complete
          log.innerHTML +=
            '<div class="log-info" style="margin-top:20px; border-top:1px solid #333; padding-top:10px;">--- Done ---</div>';
          showToast('Fertig', 'Der Housekeeping-Prozess wurde abgeschlossen.', 'success', 4000);
        }
      }

      // Load settings on startup
      window.addEventListener("DOMContentLoaded", () => {
        const saved = localStorage.getItem("gitHousekeeper_settings");
        if (saved) {
          try {
            const settings = JSON.parse(saved);
            if (settings.rootPath) {
              document.getElementById("rootPath").value = settings.rootPath;
              loadFolders(); // Trigger load
            }
            // if (settings.excluded) document.getElementById('excluded').value = settings.excluded;
            if (settings.parentVersion)
              document.getElementById("parentVersion").value =
                settings.parentVersion;
            if (settings.versionBumpStrategy)
              document.getElementById("versionBumpStrategy").value =
                settings.versionBumpStrategy;
            if (settings.runCleanInstall !== undefined)
              document.getElementById("runCleanInstall").checked =
                settings.runCleanInstall;

            if (settings.branchStrategy) {
              const rb = document.querySelector(
                `input[name="branchStrategy"][value="${settings.branchStrategy}"]`
              );
              if (rb) rb.checked = true;
            }
            if (settings.customBranchName)
              document.getElementById("customBranchName").value =
                settings.customBranchName;
            toggleBranchInput();
          } catch (e) {
            console.error("Failed to load settings", e);
          }
        }

        // Auto-load folders when typing/pasting path
        const rootPathInput = document.getElementById("rootPath");
        let debounceTimer;
        rootPathInput.addEventListener("input", () => {
          clearTimeout(debounceTimer);
          debounceTimer = setTimeout(() => {
            loadFolders();
          }, 500);
        });

        // Dashboard Logic
        const savedSettings = localStorage.getItem("gitHousekeeper_settings");
        if (savedSettings) {
          try {
            const settings = JSON.parse(savedSettings);
            if (settings.rootPath) {
              document.getElementById("rootPath").value = settings.rootPath;
              loadDashboardStats(settings.rootPath);
              // Also trigger folder load for settings tab
              loadFolders();
            } else {
               // No path, show empty state
               document.getElementById("dashboard-empty").classList.remove("hidden");
               document.getElementById("dashboard-path-header").classList.add("hidden");
            }
          } catch (e) {
            console.error("Error loading settings for dashboard", e);
          }
        } else {
           document.getElementById("dashboard-empty").classList.remove("hidden");
           document.getElementById("dashboard-path-header").classList.add("hidden");
        }
      });

      async function loadDashboardStats(rootPath) {
        lastLoadedPath = rootPath;
        const content = document.getElementById("dashboard-content");
        const empty = document.getElementById("dashboard-empty");
        const header = document.getElementById("dashboard-path-header");
        const tbody = document.getElementById("repo-table-body");

        empty.classList.add("hidden");
        header.classList.remove("hidden");
        content.classList.remove("hidden");

        // Reset UI
        tbody.innerHTML = "";
        document.getElementById("metric-health").innerText = "--";
        document.getElementById("metric-repos").innerText = "0";
        document.getElementById("metric-todos").innerText = "0";
        document.getElementById("chart-deps").innerHTML = '<div class="hint">Loading...</div>';
        document.getElementById("chart-spring").innerHTML = '<div class="hint">Loading...</div>';

        // Reset Stats
        currentStats = {
            totalRepos: 0,
            repoDetails: [],
            springVersions: {},
            topDependencies: {}, // Map for easy counting
            totalTodos: 0,
            totalHealth: 0
        };

        try {
          const response = await fetch("/api/dashboard-stats", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ RootPath: rootPath, Excluded: [] })
          });

          if (!response.ok) throw new Error("Failed to load stats");

          const reader = response.body.getReader();
          const decoder = new TextDecoder();
          let buffer = "";

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            buffer += decoder.decode(value, { stream: true });
            const lines = buffer.split("\n");
            buffer = lines.pop(); // Keep incomplete line

            for (const line of lines) {
                if (!line.trim()) continue;
                try {
                    const msg = JSON.parse(line);
                    handleDashboardMessage(msg);
                } catch (e) {
                    console.error("Error parsing JSON", e);
                }
            }
          }
        } catch (e) {
          console.error(e);
          // content.classList.add("hidden");
        }
      }

      function handleDashboardMessage(msg) {
        if (msg.type === "init") {
            currentStats.totalRepos = msg.totalRepos;
            document.getElementById("metric-repos").innerText = msg.totalRepos;
            // Clear charts
            document.getElementById("chart-deps").innerHTML = "";
            document.getElementById("chart-spring").innerHTML = "";
        } else if (msg.type === "repo") {
            const repo = msg.data;
            const deps = msg.deps || [];

            // Update Stats
            currentStats.repoDetails.push(repo);
            currentStats.totalTodos += repo.todoCount;
            currentStats.totalHealth += repo.healthScore;

            if (repo.springBootVer) {
                currentStats.springVersions[repo.springBootVer] = (currentStats.springVersions[repo.springBootVer] || 0) + 1;
            }

            deps.forEach(d => {
                currentStats.topDependencies[d] = (currentStats.topDependencies[d] || 0) + 1;
            });

            // Update Metrics
            const count = currentStats.repoDetails.length;
            const avgHealth = Math.round(currentStats.totalHealth / count);
            document.getElementById("metric-health").innerText = avgHealth + "/100";
            document.getElementById("metric-todos").innerText = currentStats.totalTodos;

            // Add Row
            addRepoRow(repo);

            // Update Charts (Debounce could be good, but live is cool)
            updateCharts();
        } else if (msg.type === "done") {
            // Final polish if needed
        }
      }

      function addRepoRow(repo) {
        const tbody = document.getElementById("repo-table-body");
        let statusClass = "status-good";
        let statusText = "Healthy";
        if (repo.healthScore < 50) {
            statusClass = "status-bad";
            statusText = "Critical";
        } else if (repo.healthScore < 80) {
            statusClass = "status-warn";
            statusText = "Warning";
        }

        const tr = document.createElement("tr");
        tr.innerHTML = `
            <td>${repo.name}</td>
            <td>
                <div style="display:flex; align-items:center; gap:10px;">
                    <div style="flex:1; height:6px; background:#45475a; border-radius:3px; width:50px;">
                        <div style="width:${repo.healthScore}%; height:100%; background:${repo.healthScore < 50 ? '#f38ba8' : repo.healthScore < 80 ? '#fab387' : '#a6e3a1'}; border-radius:3px;"></div>
                    </div>
                    <span>${repo.healthScore}</span>
                </div>
            </td>
            <td>${repo.springBootVer || '-'}</td>
            <td>${repo.javaVersion || '-'}</td>
            <td>${repo.lastCommit || '-'}</td>
            <td>${repo.todoCount}</td>
            <td><span class="status-badge ${statusClass}">${statusText}</span></td>
        `;
        tbody.appendChild(tr);
      }

      function updateCharts() {
        // Top Dependencies
        const sortedDeps = Object.entries(currentStats.topDependencies)
            .sort((a, b) => b[1] - a[1])
            .slice(0, 5);

        const depsContainer = document.getElementById("chart-deps");
        depsContainer.innerHTML = "";

        // Find max for scaling
        const maxDep = sortedDeps.length > 0 ? sortedDeps[0][1] : 1;

        sortedDeps.forEach(([name, count]) => {
            const pct = (count / maxDep) * 100;
            const row = document.createElement("div");
            row.className = "bar-row";
            row.innerHTML = `
                <div class="bar-label" title="${name}">${name}</div>
                <div class="bar-track">
                    <div class="bar-fill" style="width: ${pct}%"></div>
                </div>
                <div class="bar-value">${count}</div>
            `;
            depsContainer.appendChild(row);
        });

        // Spring Boot Versions
        const sortedSpring = Object.entries(currentStats.springVersions)
            .sort((a, b) => b[1] - a[1]); // Sort by count desc

        const springContainer = document.getElementById("chart-spring");
        springContainer.innerHTML = "";

        if (sortedSpring.length === 0) {
            springContainer.innerHTML = '<div class="hint">No Spring Boot projects found.</div>';
        } else {
            const maxSpring = sortedSpring[0][1];
            sortedSpring.forEach(([ver, count]) => {
                const pct = (count / maxSpring) * 100;
                const row = document.createElement("div");
                row.className = "bar-row";
                row.innerHTML = `
                    <div class="bar-label">Spring Boot ${ver}</div>
                    <div class="bar-track">
                        <div class="bar-fill" style="width: ${pct}%; background-color: #89b4fa;"></div>
                    </div>
                    <div class="bar-value">${count}</div>
                `;
                springContainer.appendChild(row);
            });
        }
      }

      async function loadSpringVersions() {
        const container = document.getElementById("spring-versions-list");

        try {
          const res = await fetch("/api/spring-versions");
          if (!res.ok) throw new Error("Failed to fetch versions");
          const versions = await res.json();

          // Cache for Migration Assistant
          window.springVersionsCache = versions;

          container.innerHTML = "";

          // Show only first 5 branches initially
          const initialShowCount = 5;
          let hiddenContainer = null;
          let showMoreBtn = null;

          versions.forEach((group, index) => {
            const groupDiv = document.createElement("div");
            groupDiv.style.marginBottom = "20px";
            groupDiv.style.padding = "15px";
            groupDiv.style.backgroundColor = "var(--input-bg)";
            groupDiv.style.borderRadius = "6px";

            const title = document.createElement("h4");
            title.style.margin = "0 0 10px 0";
            title.style.color = "var(--accent-color)";
            title.textContent = `Spring Boot ${group.Branch}`;

            if (group.MigrationGuide) {
              const link = document.createElement("a");
              link.href = group.MigrationGuide;
              link.target = "_blank";
              link.style.fontSize = "0.8em";
              link.style.marginLeft = "10px";
              link.style.color = "#89b4fa";
              link.textContent = group.Branch.endsWith(".0")
                ? "📖 Migration Guide"
                : "📝 Release Notes";
              title.appendChild(link);
            }

            groupDiv.appendChild(title);

            const versionsDiv = document.createElement("div");
            versionsDiv.style.fontSize = "0.9em";
            versionsDiv.style.color = "#a6adc8";
            // Show top 5 versions, then "..."
            const showCount = 5;
            const visible = group.Versions.slice(0, showCount);
            versionsDiv.textContent = visible.join(", ");

            if (group.Versions.length > showCount) {
              const more = document.createElement("span");
              more.textContent = ` (+${
                group.Versions.length - showCount
              } weitere)`;
              more.style.cursor = "pointer";
              more.style.textDecoration = "underline";
              more.style.marginLeft = "5px";
              more.onclick = () => {
                versionsDiv.textContent = group.Versions.join(", ");
              };
              versionsDiv.appendChild(more);
            }

            groupDiv.appendChild(versionsDiv);

            // Add to visible container or hidden container based on index
            if (index < initialShowCount) {
              container.appendChild(groupDiv);
            } else {
              // Create hidden container if not exists
              if (!hiddenContainer) {
                hiddenContainer = document.createElement("div");
                hiddenContainer.id = "hidden-versions";
                hiddenContainer.style.display = "none";
                container.appendChild(hiddenContainer);
              }
              hiddenContainer.appendChild(groupDiv);
            }
          });

          // Add "Show More" button if there are hidden items
          if (versions.length > initialShowCount) {
            showMoreBtn = document.createElement("button");
            showMoreBtn.className = "btn btn-secondary";
            showMoreBtn.style.width = "100%";
            showMoreBtn.style.marginTop = "10px";
            showMoreBtn.innerHTML = `📋 Show older versions (+${
              versions.length - initialShowCount
            })`;
            showMoreBtn.onclick = () => {
              const hidden = document.getElementById("hidden-versions");
              if (hidden.style.display === "none") {
                hidden.style.display = "block";
                showMoreBtn.innerHTML = "📋 Hide older versions";
              } else {
                hidden.style.display = "none";
                showMoreBtn.innerHTML = `📋 Show older versions (+${
                  versions.length - initialShowCount
                })`;
              }
            };
            container.appendChild(showMoreBtn);
          }

          // Also populate the dropdown if we are in Spring Boot mode
          const typeInput = document.querySelector('input[name="migrationType"]:checked');
          if (typeInput && typeInput.value === 'spring-boot') {
              populateSpringVersions(versions);
          }

        } catch (e) {
          container.innerHTML = `<div class="log-error">Error: ${e.message}</div>`;
        }
      }

      function populateSpringVersions(versions) {
        const select = document.getElementById("targetBootVersion");
        if (!select) return;
        select.innerHTML = '<option value="">Select Version...</option>';

        versions.forEach((v) => {
            // Add the branch (Major.Minor) as value - OpenRewrite only has recipes for minor versions
            if (v.Versions && v.Versions.length > 0) {
                const opt = document.createElement("option");
                opt.value = v.Branch; // Use Major.Minor (e.g., "3.5") instead of full version
                opt.innerText = `Spring Boot ${v.Branch} (latest: ${v.Versions[0]})`;
                select.appendChild(opt);
            }
        });
      }

      function updateMigrationOptions() {
        const typeInput = document.querySelector('input[name="migrationType"]:checked');
        if (!typeInput) return;
        const type = typeInput.value;

        const title = document.getElementById('migration-options-title');
        const select = document.getElementById('targetBootVersion');
        const hint = document.getElementById('migration-hint');
        const card = document.getElementById('migration-options-card');
        const container = select ? select.parentElement : null;

        if (!select) return;

        // Reset
        select.innerHTML = '';
        select.style.display = 'block';
        if (container) container.style.justifyContent = '';

        if (type === 'spring-boot') {
            title.innerText = 'Target Spring Boot Version';
            hint.innerText = 'Runs OpenRewrite to upgrade Spring Boot version.';
            if (window.springVersionsCache) {
                 populateSpringVersions(window.springVersionsCache);
            } else {
                 loadSpringVersions();
            }
        } else if (type === 'java-version') {
            title.innerText = 'Target Java Version';
            hint.innerText = 'Runs OpenRewrite to upgrade Java version.';
            // LTS Versions: 25, 21, 17, 11, 8
            const versions = ['25', '21', '17', '11', '8'];
            versions.forEach(v => {
                const opt = document.createElement("option");
                opt.value = v;
                opt.innerText = 'Java ' + v + (v === '25' ? ' (Latest LTS)' : '');
                select.appendChild(opt);
            });
        } else if (type === 'jakarta-ee') {
            title.innerText = 'Jakarta EE Migration';
            hint.innerText = 'Runs OpenRewrite to migrate from javax to jakarta namespace.';
            select.style.display = 'none';
            if (container) container.style.justifyContent = 'flex-end';
        } else if (type === 'quarkus') {
            title.innerText = 'Quarkus Migration';
            hint.innerText = 'Runs OpenRewrite to migrate Quarkus 1.x to 2.x.';
            select.style.display = 'none';
            if (container) container.style.justifyContent = 'flex-end';
        }
      }

      async function checkOpenRewriteVersions() {
        const container = document.getElementById("openrewrite-versions");

        try {
          const res = await fetch("/api/openrewrite-versions");
          if (!res.ok) throw new Error(res.statusText);
          const data = await res.json();

          container.innerHTML = "";

          data.forEach((item) => {
            const card = document.createElement("div");
            card.style.cssText =
              "flex: 1; min-width: 200px; padding: 12px; background: var(--bg-color); border-radius: 6px; border: 1px solid var(--border-color);";

            const hasUpdate = item.updateAvailable;
            const statusColor = hasUpdate ? "#f9e2af" : "#a6e3a1";
            const statusIcon = hasUpdate ? "⚠️" : "✅";
            const statusText = hasUpdate ? "Update available" : "Up to date";

            card.innerHTML = `
              <div style="font-weight: 500; margin-bottom: 8px; color: var(--text-color);">
                ${item.component}
              </div>
              <div style="display: flex; justify-content: space-between; font-size: 0.9em; margin-bottom: 4px;">
                <span style="color: #6c7086;">Using:</span>
                <span style="color: var(--text-color); font-family: monospace;">${
                  item.currentVersion
                }</span>
              </div>
              <div style="display: flex; justify-content: space-between; font-size: 0.9em; margin-bottom: 8px;">
                <span style="color: #6c7086;">Latest:</span>
                <span style="color: ${
                  hasUpdate ? "#a6e3a1" : "var(--text-color)"
                }; font-family: monospace;">${item.latestVersion}</span>
              </div>
              <div style="display: flex; justify-content: space-between; align-items: center;">
                <span style="color: ${statusColor}; font-size: 0.85em;">${statusIcon} ${statusText}</span>
                <a href="${
                  item.mavenCentralUrl
                }" target="_blank" style="color: #89b4fa; font-size: 0.85em; text-decoration: none;">
                  Maven Central ↗
                </a>
              </div>
            `;

            container.appendChild(card);
          });
        } catch (e) {
          container.innerHTML = `<div class="log-error">Error: ${e.message}</div>`;
        }
      }

      async function scanSpringProjects() {
        const container = document.getElementById("spring-projects-list");
        const rootPath = document.getElementById("rootPath").value;

        // Calculate Excluded Folders from Checkboxes
        const excluded = ["node_modules", "target", "dist", ".idea", ".vscode"]; // Standard defaults
        document
          .querySelectorAll('#folder-list-container input[type="checkbox"]')
          .forEach((cb) => {
            if (!cb.checked) {
              excluded.push(cb.value);
            }
          });

        if (!rootPath) {
          alert("Please configure the project path in Project Setup first.");
          return;
        }

        container.innerHTML = '<div class="log-info">Scanning...</div>';

        try {
          const res = await fetch("/api/scan-spring", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ RootPath: rootPath, Excluded: excluded }),
          });
          if (!res.ok) throw new Error(res.statusText);
          const results = await res.json();

          container.innerHTML = "";

          // Handle new SpringScanResult structure
          if (!results || !results.Projects || results.Projects.length === 0) {
            let html = '<div class="hint">No Spring Boot projects found.</div>';

            // Show debug log if available
            if (results && results.DebugLog && results.DebugLog.length > 0) {
              html +=
                '<div style="margin-top:20px; border-top:1px solid var(--border-color); padding-top:10px;">';
              html += "<h4>Debug Log</h4>";
              html +=
                '<div style="font-family:monospace; font-size:0.8em; color:#a6adc8; max-height:200px; overflow-y:auto;">';
              results.DebugLog.forEach((line) => {
                html += `<div>${line}</div>`;
              });
              html += "</div></div>";
            }

            container.innerHTML = html;
            return;
          }

          // Display found projects
          results.Projects.forEach((proj) => {
            const div = document.createElement("div");
            div.style.padding = "10px";
            div.style.borderBottom = "1px solid var(--border-color)";

            div.innerHTML = `
                    <div style="font-weight:bold;">${proj.RepoName}</div>
                    <div style="font-size:0.9em; color: #a6adc8;">Aktuell: <span style="color: var(--accent-color);">${proj.CurrentVersion}</span></div>
                `;
            container.appendChild(div);
          });
        } catch (e) {
          container.innerHTML = `<div class="log-error">Error scanning: ${e.message}</div>`;
        }
      }

      // Folder Picker: Call backend API to open native OS dialog
      async function pickFolder(targetId) {
        try {
          const res = await fetch("/api/pick-folder");
          if (!res.ok) {
            const text = await res.text();
            throw new Error(text);
          }
          const data = await res.json();
          if (data.path) {
            document.getElementById(targetId).value = data.path;
            if (targetId === "rootPath") {
              loadFolders();
              loadDashboardStats(data.path);
            }
          }
        } catch (e) {
          console.error(e);
          alert("Error opening dialog: " + e.message);
        }
      }

      async function loadFolders() {
        const rootPath = document.getElementById("rootPath").value;
        const container = document.getElementById("folder-list-container");

        if (!rootPath) {
          container.innerHTML = "";
          return;
        }

        // Save path immediately to localStorage for better UX
        const saved = localStorage.getItem("gitHousekeeper_settings");
        let settings = {};
        if (saved) {
            try { settings = JSON.parse(saved); } catch(e) {}
        }
        settings.rootPath = rootPath;
        localStorage.setItem("gitHousekeeper_settings", JSON.stringify(settings));

        container.innerHTML = '<div class="log-info">Loading folders...</div>';

        try {
          const res = await fetch("/api/list-folders", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ Path: rootPath }),
          });

          if (!res.ok) throw new Error(res.statusText);
          const data = await res.json();

          container.innerHTML = "";

          if (data.Error) {
            container.innerHTML = `<div class="log-error">${data.Error}</div>`;
            return;
          }

          if (data.IsRepo) {
            container.innerHTML =
              '<div class="hint">None (Current folder is a Git Repository)</div>';
            return;
          }

          if (!data.Folders || data.Folders.length === 0) {
            container.innerHTML =
              '<div class="hint">No subfolders found.</div>';
            return;
          }

          const ignoreDisplay = [
            ".git",
            "node_modules",
            "target",
            "dist",
            ".idea",
            ".vscode",
          ];

          data.Folders.forEach((folder) => {
            if (ignoreDisplay.includes(folder)) return;

            const div = document.createElement("div");
            div.style.marginBottom = "5px";

            const checkbox = document.createElement("input");
            checkbox.type = "checkbox";
            checkbox.id = "folder_" + folder;
            checkbox.value = folder;
            checkbox.checked = true; // Default checked
            checkbox.style.width = "auto";
            checkbox.style.marginRight = "10px";

            const label = document.createElement("label");
            label.htmlFor = "folder_" + folder;
            label.textContent = folder;
            label.style.display = "inline";
            label.style.fontWeight = "normal";

            div.appendChild(checkbox);
            div.appendChild(label);
            container.appendChild(div);
          });

          if (container.children.length === 0) {
            container.innerHTML =
              '<div class="hint">No relevant subfolders found.</div>';
          }
        } catch (e) {
          container.innerHTML = `<div class="log-error">Error loading folders: ${e.message}</div>`;
        }
      }


      async function runOpenRewriteAnalysis() {
        const rootPath = document.getElementById("rootPath").value;
        const targetVersionSelect = document.getElementById("targetBootVersion");
        const targetVersion = targetVersionSelect ? targetVersionSelect.value : "";

        const typeInput = document.querySelector('input[name="migrationType"]:checked');
        const migrationType = typeInput ? typeInput.value : "spring-boot";

        const container = document.getElementById("migration-report-container");
        const log = document.getElementById("migration-log");
        const progressContainer = document.getElementById("migration-progress");
        const progressBar = document.getElementById("progress-bar");
        const progressText = document.getElementById("progress-text");
        const progressEta = document.getElementById("progress-eta");
        const progressPercent = document.getElementById("progress-percent");

        // Get excluded folders
        const excluded = ["node_modules", "target", "dist", ".idea", ".vscode"];
        document
          .querySelectorAll('#folder-list-container input[type="checkbox"]')
          .forEach((cb) => {
            if (!cb.checked) {
              excluded.push(cb.value);
            }
          });

        if (!rootPath) {
          alert("Please configure the project path first.");
          return;
        }

        // Validate target version only if needed
        if ((migrationType === 'spring-boot' || migrationType === 'java-version') && !targetVersion) {
          alert("Please select a target version.");
          return;
        }

        container.classList.remove("hidden");
        progressContainer.classList.add("hidden");
        log.innerHTML =
          '<div class="log-info">Starting OpenRewrite analysis... This may take a while.</div>';
        isProcessRunning = true; // Mark process as running

        let totalProjects = 0;

        try {
          const res = await fetch("/api/analyze-spring", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              RootPath: rootPath,
              Excluded: excluded,
              TargetVersion: targetVersion,
              MigrationType: migrationType
            }),
          });

          const reader = res.body.getReader();
          const decoder = new TextDecoder("utf-8");

          while (true) {
            const { done, value } = await reader.read();
            if (done) break;

            const chunk = decoder.decode(value, { stream: true });
            const lines = chunk.split("\n");

            for (let line of lines) {
              if (!line.trim()) continue;

              // Handle progress markers
              if (line.startsWith("PROGRESS_INIT:")) {
                totalProjects = parseInt(line.split(":")[1]);
                progressContainer.classList.remove("hidden");
                progressBar.style.width = "0%";
                progressText.textContent = `Analyzing... 0/${totalProjects}`;
                progressEta.textContent = "Estimated: calculating...";
                progressPercent.textContent = "0%";
                continue;
              }

              if (line.startsWith("PROGRESS_UPDATE:")) {
                const parts = line.split(":");
                const completed = parseInt(parts[1]);
                const total = parseInt(parts[2]);
                const remainingSecs = parseFloat(parts[3]);

                const percent = Math.round((completed / total) * 100);
                progressBar.style.width = percent + "%";
                progressText.textContent = `Analyzing... ${completed}/${total}`;
                progressPercent.textContent = percent + "%";

                if (remainingSecs > 0) {
                  const mins = Math.floor(remainingSecs / 60);
                  const secs = Math.round(remainingSecs % 60);
                  if (mins > 0) {
                    progressEta.textContent = `Estimated: ~${mins}m ${secs}s remaining`;
                  } else {
                    progressEta.textContent = `Estimated: ~${secs}s remaining`;
                  }
                } else {
                  progressEta.textContent = "Finishing...";
                }
                continue;
              }

              if (line.startsWith("PROGRESS_DONE:")) {
                const totalSecs = parseFloat(line.split(":")[1]);
                const mins = Math.floor(totalSecs / 60);
                const secs = Math.round(totalSecs % 60);
                progressBar.style.width = "100%";
                progressPercent.textContent = "100%";
                progressText.textContent = `Completed ${totalProjects}/${totalProjects}`;
                if (mins > 0) {
                  progressEta.textContent = `Total time: ${mins}m ${secs}s`;
                } else {
                  progressEta.textContent = `Total time: ${secs}s`;
                }
                continue;
              }

              // Check if line contains HTML (from migration summary)
              if (
                line.includes('<div class="migration-summary">') ||
                line.includes('<div class="summary-section">') ||
                line.includes("</div>")
              ) {
                // Render HTML directly
                log.innerHTML += line;
                log.scrollTop = log.scrollHeight;
                continue;
              }

              // Regular log output - escape HTML
              const formattedLine = line
                .replace(/</g, "&lt;")
                .replace(/>/g, "&gt;");

              // Color coding
              let cssClass = "log-info";
              if (line.includes("✗") || line.toLowerCase().includes("error")) {
                cssClass = "log-error";
              } else if (line.includes("✓") || line.includes(">>>")) {
                cssClass = "log-success";
              } else if (line.includes("Changes detected")) {
                cssClass = "log-warning";
              }

              log.innerHTML += `<div class="${cssClass}">${formattedLine}</div>`;
              log.scrollTop = log.scrollHeight;
            }
          }

          log.innerHTML +=
            '<div class="log-success" style="margin-top: 20px; border-top: 1px solid #444; padding-top: 10px;">--- Analysis Complete ---</div>';
          isProcessRunning = false; // Mark process as complete
          showToast('Analyse abgeschlossen', 'Die Migration-Analyse wurde erfolgreich beendet.', 'success', 4000);
        } catch (e) {
          log.innerHTML += `<div class="log-error">Error: ${e.message}</div>`;
          progressContainer.classList.add("hidden");
          isProcessRunning = false; // Mark process as complete
          showToast('Fehler', `Analyse fehlgeschlagen: ${e.message}`, 'error');
        }
      }
