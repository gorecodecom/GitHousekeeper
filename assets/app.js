// ===========================================
// Error Handling & User Feedback System
// ===========================================

let isProcessRunning = false;
let serverHealthy = true;
let healthCheckInterval = null;

// ===========================================
// Helper Functions
// ===========================================

// Format duration in seconds to human-readable string
function formatDuration(seconds) {
  if (!seconds || seconds < 0) return "calculating...";
  if (seconds < 60) return `${Math.round(seconds)}s`;
  const mins = Math.floor(seconds / 60);
  const secs = Math.round(seconds % 60);
  if (mins < 60) return `${mins}m ${secs}s`;
  const hours = Math.floor(mins / 60);
  const remainMins = mins % 60;
  return `${hours}h ${remainMins}m`;
}

// ===========================================
// Accessibility (a11y) Helpers
// ===========================================

// Keyboard Navigation for Sidebar
function handleNavKeydown(event, tabName) {
  if (event.key === "Enter" || event.key === " ") {
    event.preventDefault();
    showTab(tabName);
  }
  // Arrow key navigation
  if (event.key === "ArrowDown" || event.key === "ArrowUp") {
    event.preventDefault();
    const navItems = document.querySelectorAll('.nav-item[role="menuitem"]');
    const currentIndex = Array.from(navItems).indexOf(event.target);
    let nextIndex;
    if (event.key === "ArrowDown") {
      nextIndex = (currentIndex + 1) % navItems.length;
    } else {
      nextIndex = (currentIndex - 1 + navItems.length) % navItems.length;
    }
    navItems[nextIndex].focus();
  }
}

// Toast Notification System
function showToast(title, message, type = "info", duration = 5000) {
  const container = document.getElementById("toast-container");
  const toast = document.createElement("div");
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
      toast.classList.add("fade-out");
      setTimeout(() => toast.remove(), 300);
    }, duration);
  }

  return toast;
}

// Connection Status Banner
function showConnectionBanner(type, icon, text) {
  const banner = document.getElementById("connection-banner");
  const iconEl = document.getElementById("connection-icon");
  const textEl = document.getElementById("connection-text");

  banner.className = `visible ${type}`;
  iconEl.textContent = icon;
  textEl.textContent = text;
}

function hideConnectionBanner() {
  const banner = document.getElementById("connection-banner");
  banner.classList.remove("visible");
}

// Online/Offline Detection
window.addEventListener("offline", () => {
  showConnectionBanner(
    "offline",
    "🔴",
    "No internet connection - Some features may be limited"
  );
  showToast("Offline", "You have no internet connection.", "warning");
});

window.addEventListener("online", () => {
  showConnectionBanner("reconnected", "🟢", "Connection restored");
  showToast("Online", "Internet connection restored.", "success", 3000);
  setTimeout(hideConnectionBanner, 3000);
});

// Server Health Check
async function checkServerHealth() {
  try {
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 5000);

    const res = await fetch("/api/health", {
      method: "HEAD",
      signal: controller.signal,
    });
    clearTimeout(timeoutId);

    if (!serverHealthy) {
      // Server was down, now back up
      serverHealthy = true;
      showConnectionBanner("reconnected", "🟢", "Server connection restored");
      showToast(
        "Server connected",
        "Connection to GitHousekeeper server has been restored.",
        "success",
        3000
      );
      setTimeout(hideConnectionBanner, 3000);
    }
  } catch (e) {
    if (serverHealthy) {
      serverHealthy = false;
      showConnectionBanner(
        "server-error",
        "⚠️",
        "Server unreachable - Please restart GitHousekeeper"
      );
      showToast(
        "Server unreachable",
        "The GitHousekeeper server is not responding. Please check if the application is still running.",
        "error",
        0
      );
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
window.addEventListener("beforeunload", (event) => {
  if (isProcessRunning) {
    event.preventDefault();
    event.returnValue =
      "A process is still running. Do you really want to leave this page?";
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
    if (error.name === "TypeError" || error.message.includes("fetch")) {
      serverHealthy = false;
      showConnectionBanner("server-error", "⚠️", "Server unreachable");
      showToast(
        "Connection error",
        "The server is not reachable. Please check if GitHousekeeper is still running.",
        "error",
        0
      );
    }
    throw error;
  }
}

// Initialize on page load
document.addEventListener("DOMContentLoaded", () => {
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
  totalHealth: 0,
};
let lastLoadedPath = "";

function showTab(tabId) {
  // Update Sidebar
  document.querySelectorAll(".nav-item").forEach((el) => {
    el.classList.remove("active");
    el.removeAttribute("aria-current");
  });
  const activeNavItem = document.querySelector(
    `.nav-item[onclick="showTab('${tabId}')"]`
  );
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

  if (tabId === "maintenance") {
    loadBranchInfo();
  }

  if (tabId === "security") {
    // Auto-load repos when switching to security tab
    if (!securityReposLoaded) {
      loadSecurityRepos();
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
  el.style.height = "auto";
  el.style.height = el.scrollHeight + "px";
}

function printSection(section) {
  try {
    // For migration, we need to be on the frameworks tab
    if (section === "migration") {
      // Temporarily show the frameworks tab for printing
      const frameworksTab = document.getElementById("tab-frameworks");
      const wasHidden = !frameworksTab.classList.contains("active");
      if (wasHidden) {
        frameworksTab.style.display = "block";
      }
    }

    document.body.classList.add("print-" + section);
    window.print();
    document.body.classList.remove("print-" + section);

    // Restore tab state if needed
    if (section === "migration") {
      const frameworksTab = document.getElementById("tab-frameworks");
      if (!frameworksTab.classList.contains("active")) {
        frameworksTab.style.display = "";
      }
    }
  } catch (e) {
    console.error("Print failed:", e);
    showToast("Error", "PDF export failed. Please try again.", "error");
    document.body.classList.remove("print-" + section);
  }
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
  const scopeAll = document.querySelector(
    'input[name="replacementScope"][value="all"]'
  );
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
    targetBranch = document.getElementById("customBranchName").value.trim();
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
    versionBumpStrategy: document.getElementById("versionBumpStrategy").value,
    runCleanInstall: document.getElementById("runCleanInstall").checked,
    targetBranch: targetBranch,
    replacements: [],
    replacementScope:
      document.querySelector('input[name="replacementScope"]:checked')?.value ||
      "all",
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
    showToast("Error", `An error occurred: ${e.message}`, "error");
  } finally {
    loading.classList.add("hidden");
    isProcessRunning = false; // Mark process as complete
    log.innerHTML +=
      '<div class="log-info" style="margin-top:20px; border-top:1px solid #333; padding-top:10px;">--- Done ---</div>';
    showToast(
      "Complete",
      "The housekeeping process has finished.",
      "success",
      4000
    );
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
        document.getElementById("parentVersion").value = settings.parentVersion;
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
        document
          .getElementById("dashboard-path-header")
          .classList.add("hidden");
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
  document.getElementById("metric-outdated").innerText = "0";
  document.getElementById("chart-deps").innerHTML =
    '<div class="hint">Loading...</div>';
  document.getElementById("chart-frameworks").innerHTML =
    '<div class="hint">Loading...</div>';

  // Reset Stats
  currentStats = {
    totalRepos: 0,
    repoDetails: [],
    frameworks: {}, // Framework distribution
    topDependencies: {}, // Map for easy counting
    totalTodos: 0,
    totalHealth: 0,
    totalOutdated: 0,
  };

  try {
    const response = await fetch("/api/dashboard-stats", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ RootPath: rootPath, Excluded: [] }),
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
    document.getElementById("chart-frameworks").innerHTML = "";
  } else if (msg.type === "repo") {
    const repo = msg.data;
    const deps = msg.deps || [];

    // Update Stats
    currentStats.repoDetails.push(repo);
    currentStats.totalTodos += repo.todoCount;
    currentStats.totalHealth += repo.healthScore;
    currentStats.totalOutdated += repo.outdatedDeps || 0;

    // Track frameworks (including Spring Boot versions as separate entries)
    if (repo.framework) {
      let frameworkLabel = repo.framework;
      // Add version info for Spring Boot
      if (repo.framework === "Spring Boot" && repo.springBootVer) {
        frameworkLabel = `Spring Boot ${repo.springBootVer}`;
      }
      currentStats.frameworks[frameworkLabel] =
        (currentStats.frameworks[frameworkLabel] || 0) + 1;
    } else if (repo.projectType === "maven" && repo.springBootVer) {
      const frameworkLabel = `Spring Boot ${repo.springBootVer}`;
      currentStats.frameworks[frameworkLabel] =
        (currentStats.frameworks[frameworkLabel] || 0) + 1;
    } else if (repo.projectType && repo.projectType !== "unknown") {
      // For projects without detected framework, use project type
      const typeLabels = {
        npm: "Node.js",
        yarn: "Node.js",
        pnpm: "Node.js",
        go: "Go",
        python: "Python",
        maven: "Maven",
      };
      const frameworkLabel = typeLabels[repo.projectType] || repo.projectType;
      currentStats.frameworks[frameworkLabel] =
        (currentStats.frameworks[frameworkLabel] || 0) + 1;
    }

    deps.forEach((d) => {
      currentStats.topDependencies[d] =
        (currentStats.topDependencies[d] || 0) + 1;
    });

    // Update Metrics
    const count = currentStats.repoDetails.length;
    const avgHealth = Math.round(currentStats.totalHealth / count);
    document.getElementById("metric-health").innerText = avgHealth + "/100";
    document.getElementById("metric-todos").innerText = currentStats.totalTodos;
    document.getElementById("metric-outdated").innerText =
      currentStats.totalOutdated;

    // Add Row
    addRepoRow(repo);

    // Update Charts (Debounce could be good, but live is cool)
    updateCharts();
  } else if (msg.type === "done") {
    // Final polish if needed
  }
}

// Get framework icon/badge
function getFrameworkBadge(framework) {
  const frameworkIcons = {
    "Next.js": { icon: "▲", color: "#000" },
    "Nuxt.js": { icon: "💚", color: "#00DC82" },
    Angular: { icon: "🅰️", color: "#DD0031" },
    "Vue.js": { icon: "💚", color: "#4FC08D" },
    React: { icon: "⚛️", color: "#61DAFB" },
    Svelte: { icon: "🔥", color: "#FF3E00" },
    Express: { icon: "🚀", color: "#000" },
    Fastify: { icon: "⚡", color: "#000" },
    NestJS: { icon: "🐱", color: "#E0234E" },
    Gatsby: { icon: "💜", color: "#663399" },
    Remix: { icon: "💿", color: "#000" },
    Koa: { icon: "🥝", color: "#000" },
    Electron: { icon: "⚡", color: "#47848F" },
    "Spring Boot": { icon: "🍃", color: "#6DB33F" },
    // Go frameworks
    Go: { icon: "🐹", color: "#00ADD8" },
    Gin: { icon: "🐹", color: "#00ADD8" },
    Fiber: { icon: "🐹", color: "#00ADD8" },
    Echo: { icon: "🐹", color: "#00ADD8" },
    Chi: { icon: "🐹", color: "#00ADD8" },
    "Gorilla Mux": { icon: "🐹", color: "#00ADD8" },
    gRPC: { icon: "🐹", color: "#00ADD8" },
    // Python frameworks
    Python: { icon: "🐍", color: "#3776AB" },
    Django: { icon: "🐍", color: "#092E20" },
    Flask: { icon: "🐍", color: "#000" },
    FastAPI: { icon: "🐍", color: "#009688" },
    Streamlit: { icon: "🐍", color: "#FF4B4B" },
    PyTorch: { icon: "🔥", color: "#EE4C2C" },
    TensorFlow: { icon: "🧠", color: "#FF6F00" },
    "Data Science": { icon: "📊", color: "#3776AB" },
    // PHP frameworks
    PHP: { icon: "🐘", color: "#777BB4" },
    Laravel: { icon: "🐘", color: "#FF2D20" },
    Symfony: { icon: "🐘", color: "#000" },
    CodeIgniter: { icon: "🐘", color: "#EE4623" },
    CakePHP: { icon: "🐘", color: "#D33C44" },
    Yii: { icon: "🐘", color: "#40B3D8" },
    Slim: { icon: "🐘", color: "#74A045" },
  };

  const info = frameworkIcons[framework] || { icon: "📦", color: "#888" };
  return `<span style="color: ${info.color};" title="${framework}">${info.icon}</span>`;
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

  // Build Framework column
  let frameworkDisplay = "-";
  if (repo.framework) {
    frameworkDisplay = `${getFrameworkBadge(repo.framework)} ${repo.framework}`;
    if (repo.framework === "Spring Boot" && repo.springBootVer) {
      frameworkDisplay += ` ${repo.springBootVer}`;
    }
  } else if (repo.springBootVer) {
    frameworkDisplay = `${getFrameworkBadge("Spring Boot")} Spring Boot ${
      repo.springBootVer
    }`;
  }

  // Build Runtime column (Java, Node.js, Go, or Python version)
  let runtimeDisplay = "-";
  if (repo.javaVersion) {
    runtimeDisplay = `☕ Java ${repo.javaVersion}`;
  } else if (repo.nodeVersion) {
    runtimeDisplay = `📗 Node ${repo.nodeVersion}`;
  } else if (repo.goVersion) {
    runtimeDisplay = `🐹 Go ${repo.goVersion}`;
  } else if (repo.pythonVersion) {
    runtimeDisplay = `🐍 Python ${repo.pythonVersion}`;
  }

  // Outdated deps display
  let outdatedDisplay = repo.outdatedDeps || 0;
  let outdatedClass =
    outdatedDisplay === 0
      ? "status-good"
      : outdatedDisplay > 10
      ? "status-bad"
      : "status-warn";
  let outdatedBadge =
    outdatedDisplay === 0 ? "✅" : outdatedDisplay > 10 ? "⚠️" : "📦";

  const tr = document.createElement("tr");
  tr.innerHTML = `
            <td>${repo.name}</td>
            <td>
                <div style="display:flex; align-items:center; gap:10px;">
                    <div style="flex:1; height:6px; background:#45475a; border-radius:3px; width:50px;">
                        <div style="width:${
                          repo.healthScore
                        }%; height:100%; background:${
    repo.healthScore < 50
      ? "#f38ba8"
      : repo.healthScore < 80
      ? "#fab387"
      : "#a6e3a1"
  }; border-radius:3px;"></div>
                    </div>
                    <span>${repo.healthScore}</span>
                </div>
            </td>
            <td>${frameworkDisplay}</td>
            <td>${runtimeDisplay}</td>
            <td>${repo.lastCommit || "-"}</td>
            <td>${repo.todoCount}</td>
            <td><span title="${outdatedDisplay} outdated packages">${outdatedBadge} ${outdatedDisplay}</span></td>
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

  if (sortedDeps.length === 0) {
    depsContainer.innerHTML =
      '<div class="hint" style="color: #a6adc8; font-size: 0.9em;">Dependencies are collected during scan. Maven, Node.js, Go and Python projects are supported.</div>';
  } else {
    // Find max for scaling
    const maxDep = sortedDeps[0][1];

    sortedDeps.forEach(([name, count]) => {
      const pct = (count / maxDep) * 100;
      const tooltip = `${name} is used in ${count} project${
        count > 1 ? "s" : ""
      }`;
      const row = document.createElement("div");
      row.className = "bar-row";
      row.style.cursor = "help";
      row.title = tooltip;
      row.innerHTML = `
                    <div class="bar-label" title="${tooltip}">${name}</div>
                    <div class="bar-track" title="${tooltip}">
                        <div class="bar-fill" style="width: ${pct}%; background-color: #89b4fa;"></div>
                    </div>
                    <div class="bar-value">${count}</div>
                `;
      depsContainer.appendChild(row);
    });
  }

  // Frameworks Chart
  const sortedFrameworks = Object.entries(currentStats.frameworks).sort(
    (a, b) => b[1] - a[1]
  ); // Sort by count desc

  const frameworksContainer = document.getElementById("chart-frameworks");
  frameworksContainer.innerHTML = "";

  if (sortedFrameworks.length === 0) {
    frameworksContainer.innerHTML =
      '<div class="hint" style="color: #a6adc8; font-size: 0.9em;">No frameworks detected. Supported: Spring Boot, React, Vue, Angular, Next.js, Go, Python, Django, Flask, etc.</div>';
  } else {
    const maxFramework = sortedFrameworks[0][1];

    sortedFrameworks.forEach(([name, count]) => {
      const pct = (count / maxFramework) * 100;
      const tooltip = `${count} project${count > 1 ? "s" : ""} use${
        count > 1 ? "" : "s"
      } ${name}`;

      const row = document.createElement("div");
      row.className = "bar-row";
      row.style.cursor = "help";
      row.title = tooltip;
      row.innerHTML = `
                    <div class="bar-label" title="${tooltip}">${name}</div>
                    <div class="bar-track" title="${tooltip}">
                        <div class="bar-fill" style="width: ${pct}%; background-color: #a6e3a1;"></div>
                    </div>
                    <div class="bar-value">${count}</div>
                `;
      frameworksContainer.appendChild(row);
    });
  }
}

// Track if Spring versions have been loaded to avoid redundant fetches
let springVersionsLoaded = false;

async function loadSpringVersions() {
  const container = document.getElementById("spring-versions-list");

  // Skip if already loaded (use cache)
  if (springVersionsLoaded && window.springVersionsCache) {
    return;
  }

  // Show loading indicator
  container.innerHTML =
    '<div class="hint">Loading Spring Boot versions from Maven Central...</div>';

  try {
    const res = await fetch("/api/spring-versions");
    if (!res.ok) throw new Error("Failed to fetch versions");
    const versions = await res.json();

    // Cache for Migration Assistant
    window.springVersionsCache = versions;
    springVersionsLoaded = true;

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
        more.textContent = ` (+${group.Versions.length - showCount} weitere)`;
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
    const typeInput = document.querySelector(
      'input[name="migrationType"]:checked'
    );
    if (typeInput && typeInput.value === "spring-boot") {
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
  const typeInput = document.querySelector(
    'input[name="migrationType"]:checked'
  );
  if (!typeInput) return;
  const type = typeInput.value;

  const title = document.getElementById("migration-options-title");
  const select = document.getElementById("targetBootVersion");
  const hint = document.getElementById("migration-hint");
  const card = document.getElementById("migration-options-card");
  const container = select ? select.parentElement : null;

  if (!select) return;

  // Reset
  select.innerHTML = "";
  select.style.display = "block";
  if (container) container.style.justifyContent = "";

  if (type === "spring-boot") {
    title.innerText = "Target Spring Boot Version";
    hint.innerText = "Runs OpenRewrite to upgrade Spring Boot version.";
    if (window.springVersionsCache) {
      populateSpringVersions(window.springVersionsCache);
    } else {
      loadSpringVersions();
    }
  } else if (type === "java-version") {
    title.innerText = "Target Java Version";
    hint.innerText = "Runs OpenRewrite to upgrade Java version.";
    // LTS Versions: 25, 21, 17, 11, 8
    const versions = ["25", "21", "17", "11", "8"];
    versions.forEach((v) => {
      const opt = document.createElement("option");
      opt.value = v;
      opt.innerText = "Java " + v + (v === "25" ? " (Latest LTS)" : "");
      select.appendChild(opt);
    });
  } else if (type === "jakarta-ee") {
    title.innerText = "Jakarta EE Migration";
    hint.innerText =
      "Runs OpenRewrite to migrate from javax to jakarta namespace.";
    select.style.display = "none";
    if (container) container.style.justifyContent = "flex-end";
  } else if (type === "quarkus") {
    title.innerText = "Quarkus Migration";
    hint.innerText = "Runs OpenRewrite to migrate Quarkus 1.x to 2.x.";
    select.style.display = "none";
    if (container) container.style.justifyContent = "flex-end";
  }
}

// Track if OpenRewrite versions have been loaded
let openRewriteVersionsLoaded = false;

async function checkOpenRewriteVersions() {
  // Skip if already loaded
  if (openRewriteVersionsLoaded) {
    return;
  }

  const container = document.getElementById("openrewrite-versions");

  try {
    const res = await fetch("/api/openrewrite-versions");
    if (!res.ok) throw new Error(res.statusText);
    const data = await res.json();

    openRewriteVersionsLoaded = true;
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
        // Reset security repos when path changes
        securityReposLoaded = false;
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
    try {
      settings = JSON.parse(saved);
    } catch (e) {}
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
      container.innerHTML = '<div class="hint">No subfolders found.</div>';
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

  const typeInput = document.querySelector(
    'input[name="migrationType"]:checked'
  );
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
  if (
    (migrationType === "spring-boot" || migrationType === "java-version") &&
    !targetVersion
  ) {
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
        MigrationType: migrationType,
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
          // Clear repo status list
          const repoStatusItems = document.getElementById("repo-status-items");
          if (repoStatusItems) {
            repoStatusItems.innerHTML = "";
          }
          continue;
        }

        // Handle individual repo queued (show as "running" with animated progress)
        if (line.startsWith("REPO_QUEUED:")) {
          const repoName = line.split(":")[1];
          const repoStatusItems = document.getElementById("repo-status-items");
          if (repoStatusItems) {
            const repoItem = document.createElement("div");
            repoItem.id = `repo-status-${repoName}`;
            repoItem.className = "repo-card running";
            repoItem.innerHTML = `
                    <div class="repo-card-header">
                      <span class="icon">🔄</span>
                      <span class="name">${repoName}</span>
                      <span class="status running">analyzing...</span>
                    </div>
                    <div class="repo-progress-bar">
                      <div class="fill running"></div>
                    </div>
                  `;
            repoStatusItems.appendChild(repoItem);
          }
          continue;
        }

        // Handle individual repo done
        if (line.startsWith("REPO_DONE:")) {
          const parts = line.split(":");
          const repoName = parts[1];
          const status = parts[2]; // SUCCESS or FAILED
          const duration = parseFloat(parts[3]);

          const repoItem = document.getElementById(`repo-status-${repoName}`);
          if (repoItem) {
            const isSuccess = status === "SUCCESS";
            const icon = isSuccess ? "✅" : "❌";
            const statusClass = isSuccess ? "success" : "failed";

            repoItem.className = `repo-card ${statusClass}`;
            repoItem.innerHTML = `
                    <div class="repo-card-header">
                      <span class="icon">${icon}</span>
                      <span class="name">${repoName}</span>
                      <span class="status ${statusClass}">${duration.toFixed(
              1
            )}s</span>
                    </div>
                    <div class="repo-progress-bar">
                      <div class="fill ${statusClass}"></div>
                    </div>
                  `;
          }
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
        const formattedLine = line.replace(/</g, "&lt;").replace(/>/g, "&gt;");

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
    showToast(
      "Analysis complete",
      "Migration analysis has finished successfully.",
      "success",
      4000
    );
  } catch (e) {
    log.innerHTML += `<div class="log-error">Error: ${e.message}</div>`;
    progressContainer.classList.add("hidden");
    isProcessRunning = false; // Mark process as complete
    showToast("Error", `Analysis failed: ${e.message}`, "error");
  }
}

// ==================== MAINTENANCE TAB ====================

function updateBranchStatus(
  repoName,
  branchName,
  statusText,
  statusColor,
  showDeleteBtn = false
) {
  const branchRow = document.querySelector(
    `div[data-repo="${repoName}"][data-branch="${branchName}"]`
  );
  if (branchRow) {
    const statusSpan = branchRow.querySelector(".branch-status");
    if (statusSpan) {
      statusSpan.textContent = statusText;
      statusSpan.style.color = statusColor;
    }
    if (showDeleteBtn && !branchRow.querySelector("button")) {
      const repoCard = document.querySelector(
        `div[data-repo="${repoName}"]:not([data-branch])`
      );
      const repoPath = repoCard ? repoCard.dataset.path : "";
      statusSpan.insertAdjacentHTML(
        "afterend",
        `<button onclick="deleteLocalBranch('${repoPath}', '${branchName}')"
           style="margin-left: 8px; padding: 2px 6px; font-size: 0.75em; background: #ef5350; color: white; border: none; border-radius: 4px; cursor: pointer;"
           title="Delete local branch">🗑️</button>`
      );
    }
  }
}

function getExcludedProjects() {
  const excluded = ["node_modules", "target", "dist", ".idea", ".vscode"];
  document
    .querySelectorAll('#folder-list-container input[type="checkbox"]')
    .forEach((cb) => {
      if (!cb.checked) {
        excluded.push(cb.value);
      }
    });
  return excluded;
}

async function loadBranchInfo() {
  const rootPath = document.getElementById("rootPath")?.value;
  if (!rootPath) {
    showToast(
      "Error",
      "Please configure a root path in Project Setup first.",
      "error"
    );
    return;
  }

  const pathDisplay = document.getElementById("maintenance-root-path");
  const container = document.getElementById("maintenance-repos-container");

  pathDisplay.textContent = rootPath;
  container.innerHTML =
    '<div style="color: #9ca0b0; grid-column: 1 / -1; text-align: center; padding: 40px;">Loading...</div>';

  try {
    const excluded = getExcludedProjects();
    const response = await fetch("/api/list-branches", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ rootPath, excluded }),
    });

    if (!response.ok) throw new Error("Failed to load branches");

    const repos = await response.json();

    if (!repos || repos.length === 0) {
      container.innerHTML =
        '<div style="color: #9ca0b0; grid-column: 1 / -1; text-align: center; padding: 40px;">No repositories found</div>';
      return;
    }

    container.innerHTML = repos
      .map(
        (repo) => `
            <div style="background-color: var(--input-bg); border-radius: 8px; padding: 15px; border: 1px solid var(--border-color);" data-repo="${
              repo.name
            }" data-path="${repo.path}">
              <div style="display: flex; align-items: center; margin-bottom: 10px;">
                <span style="font-size: 1.2em; margin-right: 8px;">📁</span>
                <span style="font-weight: bold; color: var(--accent-color);">${
                  repo.name
                }</span>
                <span style="margin-left: auto; font-size: 0.8em; color: #9ca0b0; background: rgba(166, 227, 161, 0.1); padding: 2px 8px; border-radius: 4px;">${
                  repo.defaultBranch
                }</span>
              </div>
              <div style="font-size: 0.85em;">
                ${repo.branches
                  .map((branch) => {
                    const isDefault = branch.name === repo.defaultBranch;
                    const trackingIcon = branch.isTracking ? "🔗" : "📍";
                    let statusColor = "#9ca0b0";
                    let statusText = "local only";

                    if (branch.isTracking) {
                      if (branch.remoteGone) {
                        statusColor = "#ef5350";
                        statusText = "remote deleted";
                      } else {
                        statusText = "pending";
                      }
                    }

                    const deleteBtn =
                      branch.remoteGone && !isDefault
                        ? `<button onclick="deleteLocalBranch('${repo.path}', '${branch.name}')"
                           style="margin-left: 8px; padding: 2px 6px; font-size: 0.75em; background: #ef5350; color: white; border: none; border-radius: 4px; cursor: pointer;"
                           title="Delete local branch">🗑️</button>`
                        : "";

                    return `
                    <div style="display: flex; align-items: center; padding: 6px 0; border-bottom: 1px solid var(--border-color);" data-repo="${
                      repo.name
                    }" data-branch="${branch.name}" data-path="${repo.path}">
                      <span style="margin-right: 8px;">${trackingIcon}</span>
                      <span style="flex: 1; ${
                        isDefault ? "font-weight: bold;" : ""
                      }">${branch.name}</span>
                      <span class="branch-status" style="color: ${statusColor}; font-size: 0.85em;">${statusText}</span>
                      ${deleteBtn}
                    </div>
                  `;
                  })
                  .join("")}
              </div>
            </div>
          `
      )
      .join("");

    showToast("Loaded", `${repos.length} repositories found`, "success", 2000);
  } catch (e) {
    container.innerHTML = `<div style="color: #ef5350; grid-column: 1 / -1; text-align: center; padding: 40px;">Error: ${e.message}</div>`;
    showToast("Error", e.message, "error");
  }
}

async function deleteLocalBranch(repoPath, branchName) {
  if (!confirm(`Delete local branch "${branchName}"?`)) {
    return;
  }

  try {
    const response = await fetch("/api/delete-branch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ repoPath, branchName }),
    });

    const result = await response.json();

    if (!response.ok) {
      throw new Error(result.error || "Failed to delete branch");
    }

    showToast("Deleted", `Branch "${branchName}" removed`, "success", 2000);
    loadBranchInfo();
  } catch (e) {
    showToast("Error", e.message, "error");
  }
}

async function syncAllBranches() {
  const rootPath = document.getElementById("rootPath")?.value;
  if (!rootPath) {
    showToast(
      "Error",
      "Please configure a root path in Project Setup first.",
      "error"
    );
    return;
  }

  const btn = document.getElementById("sync-branches-btn");
  const progressContainer = document.getElementById("sync-progress");
  const progressBar = document.getElementById("sync-progress-bar");
  const progressText = document.getElementById("sync-progress-text");
  const progressPercent = document.getElementById("sync-progress-percent");
  const syncLog = document.getElementById("sync-log");
  const statusSpan = document.getElementById("sync-status");

  btn.disabled = true;
  btn.textContent = "⏳ Syncing...";
  progressContainer.classList.remove("hidden");
  syncLog.classList.remove("hidden");
  syncLog.innerHTML = "";
  isProcessRunning = true;

  try {
    const excluded = getExcludedProjects();
    const response = await fetch("/api/sync-branches", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ rootPath, excluded }),
    });

    const reader = response.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.trim()) continue;

        if (line.startsWith("SYNC_INIT:")) {
          const total = parseInt(line.split(":")[1]);
          progressText.textContent = `Syncing... 0/${total}`;
          progressPercent.textContent = "0%";
          progressBar.style.width = "0%";
          progressBar.setAttribute("aria-valuenow", "0");
          continue;
        }

        if (line.startsWith("SYNC_PROGRESS:")) {
          const parts = line.split(":");
          const current = parseInt(parts[1]);
          const total = parseInt(parts[2]);
          const percent = Math.round((current / total) * 100);
          progressText.textContent = `Syncing... ${current}/${total}`;
          progressPercent.textContent = `${percent}%`;
          progressBar.style.width = `${percent}%`;
          progressBar.setAttribute("aria-valuenow", percent.toString());
          continue;
        }

        if (line.startsWith("REPO_START:")) {
          const repoName = line.split(":")[1];
          syncLog.innerHTML += `<div style="color: #7c8aff; margin-top: 10px; font-weight: bold;">▶ ${repoName}</div>`;
          syncLog.scrollTop = syncLog.scrollHeight;
          continue;
        }

        if (line.startsWith("BRANCH_SYNCED:")) {
          const parts = line.split(":");
          const repoName = parts[1];
          const branchName = parts[2];
          updateBranchStatus(repoName, branchName, "synced", "#4caf50");
          continue;
        }

        if (line.startsWith("BRANCH_ERROR:")) {
          const parts = line.split(":");
          const repoName = parts[1];
          const branchName = parts[2];
          const errorMsg = parts.slice(3).join(":");
          updateBranchStatus(repoName, branchName, "error", "#ef5350");
          continue;
        }

        if (line.startsWith("BRANCH_GONE:")) {
          const parts = line.split(":");
          const repoName = parts[1];
          const branchName = parts[2];
          updateBranchStatus(
            repoName,
            branchName,
            "remote deleted",
            "#ef5350",
            true
          );
          continue;
        }

        if (line.startsWith("REPO_DONE:")) {
          continue;
        }

        if (line.startsWith("SYNC_COMPLETE")) {
          syncLog.innerHTML +=
            '<div style="color: #4caf50; margin-top: 15px; border-top: 1px solid #444; padding-top: 10px;">✓ Sync Complete</div>';
          statusSpan.textContent = "Last sync: just now";
          progressBar.setAttribute("aria-valuenow", "100");
          continue;
        }

        // Regular log line
        let cssClass = "color: #e0e0e0;";
        if (line.includes("✓")) cssClass = "color: #4caf50;";
        if (line.includes("[WARNING]")) cssClass = "color: #fab387;";

        syncLog.innerHTML += `<div style="${cssClass}">${line}</div>`;
        syncLog.scrollTop = syncLog.scrollHeight;
      }
    }

    showToast(
      "Sync complete",
      "All branches have been updated.",
      "success",
      3000
    );
  } catch (e) {
    syncLog.innerHTML += `<div style="color: #ef5350;">Error: ${e.message}</div>`;
    showToast("Error", e.message, "error");
  } finally {
    btn.disabled = false;
    btn.textContent = "⬇️ Sync All Tracked Branches";
    isProcessRunning = false;
  }
}

// ===========================================
// Security Scanner Functions
// ===========================================

let securityScanResults = [];
let trivyAvailable = false;
let trivyCheckDone = false;
let securityReposLoaded = false;

// Load repositories for security scanning
async function loadSecurityRepos() {
  const rootPath = document.getElementById("rootPath")?.value?.trim();
  const container = document.getElementById("security-repos-container");
  const pathDisplay = document.getElementById("security-root-path");

  if (!rootPath) {
    container.innerHTML = `
            <div style="color: #fab387; text-align: center; padding: 20px;">
              ⚠️ No root path configured. Please set a root path in the Dashboard first.
            </div>`;
    pathDisplay.textContent = "";
    return;
  }

  pathDisplay.textContent = rootPath;
  container.innerHTML = `
          <div style="display: flex; align-items: center; justify-content: center; gap: 10px; padding: 20px;">
            <span class="spinner" style="width: 16px; height: 16px; border-width: 2px;"></span>
            <span style="color: #9ca0b0;">Discovering repositories...</span>
          </div>`;

  try {
    // Use same endpoint as folder list in Project Setup
    // Note: API expects "Path" with capital P (Go struct)
    const res = await fetch("/api/list-folders", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ Path: rootPath }),
    });

    const data = await res.json();

    if (data.Error) {
      container.innerHTML = `
              <div style="color: #f38ba8; text-align: center; padding: 20px;">
                ${data.Error}
              </div>`;
      return;
    }

    // If the root path itself is a git repo
    if (data.IsRepo) {
      container.innerHTML = `
              <div style="color: #a6e3a1; text-align: center; padding: 20px;">
                ✓ Root path is a single Git repository
              </div>`;
      securityReposLoaded = true;
      // Still load branches for single repo
      await loadSecurityBranches();
      return;
    }

    // API returns "Folders" with capital F (Go struct)
    const allFolders = data.Folders || [];

    // Filter out standard ignored folders (same as Project Setup)
    const ignoreDisplay = [
      ".git",
      "node_modules",
      "target",
      "dist",
      ".idea",
      ".vscode",
    ];
    const repos = allFolders.filter((f) => !ignoreDisplay.includes(f));

    const excluded = getExcludedProjects();

    if (repos.length === 0) {
      container.innerHTML = `
              <div style="color: #9ca0b0; text-align: center; padding: 20px;">
                No repositories found in the specified path.
              </div>`;
      return;
    }

    // Filter out excluded repos (unchecked in Project Setup)
    const includedRepos = repos.filter((r) => !excluded.includes(r));

    let html = `<div style="margin-bottom: 10px; color: #a6e3a1; font-size: 0.9em;">
            ✓ Found ${includedRepos.length} repositories (${
      repos.length - includedRepos.length
    } excluded)
          </div>`;
    html += `<div style="display: grid; grid-template-columns: repeat(auto-fill, minmax(200px, 1fr)); gap: 8px;">`;

    for (const repo of includedRepos) {
      html += `<div style="padding: 6px 10px; background: var(--card-bg); border-radius: 4px; font-size: 0.9em; display: flex; align-items: center; gap: 6px;">
              <span style="color: #89b4fa;">📁</span>
              <span style="color: #cdd6f4;">${repo}</span>
            </div>`;
    }
    html += `</div>`;

    container.innerHTML = html;
    securityReposLoaded = true;

    // Also load available branches for the dropdown
    await loadSecurityBranches();
  } catch (e) {
    container.innerHTML = `
            <div style="color: #f38ba8; text-align: center; padding: 20px;">
              Error loading repositories: ${e.message}
            </div>`;
  }
}

// Load available branches for security scan dropdown
async function loadSecurityBranches() {
  const rootPath = document.getElementById("rootPath")?.value?.trim();
  const branchSelect = document.getElementById("security-branch-select");

  console.log(
    "[loadSecurityBranches] rootPath:",
    rootPath,
    "branchSelect:",
    branchSelect
  );

  if (!rootPath || !branchSelect) {
    console.log(
      "[loadSecurityBranches] Early return - missing rootPath or branchSelect"
    );
    return;
  }

  // Keep current selection
  const currentSelection = branchSelect.value;

  try {
    const excluded = getExcludedProjects();
    console.log("[loadSecurityBranches] Fetching branches for:", rootPath);
    const response = await fetch("/api/list-branches", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ rootPath, excluded }),
    });

    if (!response.ok) throw new Error("Failed to load branches");

    const repos = await response.json();
    console.log("[loadSecurityBranches] Got repos:", repos);

    // Collect all unique branches across all repos
    const branchSet = new Set();
    const defaultBranches = new Set();

    for (const repo of repos) {
      if (repo.defaultBranch) {
        defaultBranches.add(repo.defaultBranch);
      }
      for (const branch of repo.branches || []) {
        branchSet.add(branch.name);
      }
    }

    console.log(
      "[loadSecurityBranches] Found branches:",
      Array.from(branchSet)
    );

    // Sort branches: default branches first, then alphabetically
    const allBranches = Array.from(branchSet).sort((a, b) => {
      const aIsDefault = defaultBranches.has(a);
      const bIsDefault = defaultBranches.has(b);
      if (aIsDefault && !bIsDefault) return -1;
      if (!aIsDefault && bIsDefault) return 1;
      return a.localeCompare(b);
    });

    // Build options HTML
    let optionsHtml = '<option value="">📍 Current branch (default)</option>';

    for (const branch of allBranches) {
      const isDefault = defaultBranches.has(branch);
      const icon = isDefault ? "⭐" : "🔀";
      const selected = branch === currentSelection ? "selected" : "";
      optionsHtml += `<option value="${branch}" ${selected}>${icon} ${branch}</option>`;
    }

    branchSelect.innerHTML = optionsHtml;
  } catch (e) {
    console.error("Failed to load branches for security scan:", e);
    // Keep default option only on error
    branchSelect.innerHTML =
      '<option value="">📍 Current branch (default)</option>';
  }
}

// Check if Trivy is installed
async function checkTrivyAvailability() {
  const statusEl = document.getElementById("trivy-status");
  const statusIcon = document.getElementById("trivy-status-icon");
  const installHint = document.getElementById("trivy-install-hint");
  const recheckBtn = document.getElementById("trivy-recheck-btn");

  // Show checking state
  statusIcon.textContent = "⏳";
  statusEl.textContent = "Checking if Trivy is installed on your system...";
  statusEl.style.color = "#f9e2af";
  if (recheckBtn) recheckBtn.classList.add("hidden");

  try {
    const res = await fetch("/api/check-trivy");
    const data = await res.json();
    trivyAvailable = data.available;
    trivyCheckDone = true;

    if (trivyAvailable) {
      statusIcon.textContent = "✓";
      statusEl.innerHTML = `<span style="color: #a6e3a1;">Trivy is installed and ready (${data.version})</span>`;
      installHint.classList.add("hidden");
      if (recheckBtn) recheckBtn.classList.add("hidden");
    } else {
      statusIcon.textContent = "✗";
      statusEl.innerHTML = `<span style="color: #f38ba8;">Trivy is not installed on your system</span>`;
      installHint.classList.remove("hidden");
      if (recheckBtn) recheckBtn.classList.remove("hidden");
    }
  } catch (e) {
    console.error("Failed to check Trivy:", e);
    statusIcon.textContent = "⚠️";
    statusEl.innerHTML = `<span style="color: #fab387;">Could not check Trivy availability</span>`;
    trivyCheckDone = true;
    if (recheckBtn) recheckBtn.classList.remove("hidden");
  }
}

// Re-check Trivy availability (called after user installs Trivy)
function recheckTrivy() {
  trivyCheckDone = false;
  checkTrivyAvailability();
}

// Track npm check status
let npmCheckDone = false;
let npmAvailableManagers = { npm: false, yarn: false, pnpm: false };

// Check npm/yarn/pnpm availability
async function checkNpmAvailability() {
  const statusIcon = document.getElementById("npm-status-icon");
  const statusText = document.getElementById("npm-status");

  if (!statusIcon || !statusText) return;

  statusIcon.textContent = "⏳";
  statusText.textContent = "Checking available package managers...";

  try {
    const response = await fetch("/api/check-npm");
    const data = await response.json();
    npmAvailableManagers = data;

    const available = [];
    if (data.npm) available.push("npm");
    if (data.yarn) available.push("yarn");
    if (data.pnpm) available.push("pnpm");

    if (available.length > 0) {
      statusIcon.textContent = "✅";
      statusText.innerHTML = `<span style="color: #a6e3a1;">Available:</span> ${available.join(
        ", "
      )}`;
    } else {
      statusIcon.textContent = "⚠️";
      statusText.innerHTML =
        '<span style="color: #f9e2af;">No package managers found. Install npm, yarn, or pnpm.</span>';
    }
  } catch (error) {
    statusIcon.textContent = "❌";
    statusText.textContent = "Could not check package managers";
  }

  npmCheckDone = true;
}

// Handle scanner selection change
function onScannerChange() {
  const scanner = document.getElementById("security-scanner-select").value;
  const autoInfo = document.getElementById("auto-info");
  const owaspInfo = document.getElementById("owasp-info");
  const trivyInfo = document.getElementById("trivy-info");
  const npmInfo = document.getElementById("npm-info");
  const goInfo = document.getElementById("go-info");
  const pythonInfo = document.getElementById("python-info");
  const phpInfo = document.getElementById("php-info");

  // Hide all
  autoInfo.classList.add("hidden");
  owaspInfo.classList.add("hidden");
  trivyInfo.classList.add("hidden");
  npmInfo.classList.add("hidden");
  goInfo.classList.add("hidden");
  pythonInfo.classList.add("hidden");
  phpInfo.classList.add("hidden");

  // Show selected
  switch (scanner) {
    case "auto":
      autoInfo.classList.remove("hidden");
      break;
    case "owasp":
      owaspInfo.classList.remove("hidden");
      break;
    case "trivy":
      trivyInfo.classList.remove("hidden");
      if (!trivyCheckDone) {
        checkTrivyAvailability();
      }
      break;
    case "npm":
      npmInfo.classList.remove("hidden");
      if (!npmCheckDone) {
        checkNpmAvailability();
      }
      break;
    case "govulncheck":
      goInfo.classList.remove("hidden");
      if (!goCheckDone) {
        checkGoAvailability();
      }
      break;
    case "pip-audit":
      pythonInfo.classList.remove("hidden");
      if (!pythonCheckDone) {
        checkPythonAvailability();
      }
      break;
    case "composer-audit":
      phpInfo.classList.remove("hidden");
      if (!phpCheckDone) {
        checkPhpAvailability();
      }
      break;
  }
}

// Go availability check
let goCheckDone = false;
async function checkGoAvailability() {
  const statusIcon = document.getElementById("go-status-icon");
  const statusText = document.getElementById("go-status");
  const installHint = document.getElementById("go-install-hint");

  statusIcon.textContent = "⏳";
  statusText.textContent = "Checking govulncheck availability...";
  installHint.classList.add("hidden");

  try {
    const res = await fetch("/api/check-go");
    const data = await res.json();

    if (data.available) {
      statusIcon.textContent = "✅";
      statusText.textContent = `govulncheck installed (${
        data.version || "available"
      })`;
      installHint.classList.add("hidden");
    } else {
      statusIcon.textContent = "❌";
      statusText.textContent = "govulncheck not found";
      installHint.classList.remove("hidden");
    }
  } catch (e) {
    statusIcon.textContent = "⚠️";
    statusText.textContent = "Could not check govulncheck";
  }

  goCheckDone = true;
}

// Python availability check
let pythonCheckDone = false;
async function checkPythonAvailability() {
  const statusIcon = document.getElementById("python-status-icon");
  const statusText = document.getElementById("python-status");
  const installHint = document.getElementById("python-install-hint");

  statusIcon.textContent = "⏳";
  statusText.textContent = "Checking pip-audit availability...";
  installHint.classList.add("hidden");

  try {
    const res = await fetch("/api/check-python");
    const data = await res.json();

    if (data.available) {
      statusIcon.textContent = "✅";
      statusText.textContent = `pip-audit installed (${
        data.version || "available"
      })`;
      installHint.classList.add("hidden");
    } else {
      statusIcon.textContent = "❌";
      statusText.textContent = "pip-audit not found";
      installHint.classList.remove("hidden");
    }
  } catch (e) {
    statusIcon.textContent = "⚠️";
    statusText.textContent = "Could not check pip-audit";
  }

  pythonCheckDone = true;
}

// PHP availability check
let phpCheckDone = false;
async function checkPhpAvailability() {
  const statusIcon = document.getElementById("php-status-icon");
  const statusText = document.getElementById("php-status");
  const installHint = document.getElementById("php-install-hint");

  statusIcon.textContent = "⏳";
  statusText.textContent = "Checking Composer availability...";
  installHint.classList.add("hidden");

  try {
    const res = await fetch("/api/check-php");
    const data = await res.json();

    if (data.available) {
      statusIcon.textContent = "✅";
      statusText.textContent = `Composer installed (${
        data.version || "available"
      })`;
      installHint.classList.add("hidden");
    } else {
      statusIcon.textContent = "❌";
      statusText.textContent = "Composer not found";
      installHint.classList.remove("hidden");
    }
  } catch (e) {
    statusIcon.textContent = "⚠️";
    statusText.textContent = "Could not check Composer";
  }

  phpCheckDone = true;
}

// Get severity color
function getSeverityColor(severity) {
  switch ((severity || "").toUpperCase()) {
    case "CRITICAL":
      return "#f38ba8";
    case "HIGH":
      return "#fab387";
    case "MEDIUM":
      return "#f9e2af";
    case "LOW":
      return "#a6adc8";
    default:
      return "#9ca0b0";
  }
}

// Get severity badge HTML
function getSeverityBadge(severity) {
  const color = getSeverityColor(severity);
  return `<span style="background: ${color}22; color: ${color}; padding: 2px 8px; border-radius: 4px; font-size: 0.75em; font-weight: bold;">${severity}</span>`;
}

// Run security scan
async function runSecurityScan() {
  if (isProcessRunning) {
    showToast(
      "Process running",
      "Please wait until the current process is complete.",
      "warning"
    );
    return;
  }

  const rootPath = document.getElementById("rootPath").value.trim();
  if (!rootPath) {
    showToast("Path missing", "Please select a root path first.", "warning");
    return;
  }

  const scanner = document.getElementById("security-scanner-select").value;

  if (scanner === "trivy" && !trivyAvailable) {
    showToast(
      "Trivy not available",
      "Please install Trivy or select OWASP.",
      "warning"
    );
    return;
  }

  isProcessRunning = true;
  securityScanResults = [];

  const btn = document.getElementById("security-scan-btn");
  const exportBtn = document.getElementById("security-export-btn");
  const progressDiv = document.getElementById("security-progress");
  const progressBar = document.getElementById("security-progress-bar");
  const progressText = document.getElementById("security-progress-text");
  const progressPercent = document.getElementById("security-progress-percent");
  const progressEta = document.getElementById("security-progress-eta");
  const repoList = document.getElementById("security-repo-list");
  const resultsDiv = document.getElementById("security-results");
  const summaryDiv = document.getElementById("security-summary");

  btn.disabled = true;
  btn.innerHTML = "⏳ Scanning...";
  exportBtn.disabled = true;
  progressDiv.classList.remove("hidden");
  summaryDiv.classList.add("hidden");
  repoList.innerHTML = "";
  resultsDiv.innerHTML =
    '<div style="color: #9ca0b0; grid-column: 1 / -1; text-align: center; padding: 40px;">Scanning in progress...</div>';

  let scanStartTime = Date.now();
  let totalRepos = 0;
  let scannedRepos = 0;
  let summaryStats = { critical: 0, high: 0, medium: 0, low: 0, total: 0 };

  // Get selected target branch
  const targetBranch =
    document.getElementById("security-branch-select")?.value || "";

  try {
    const res = await fetch("/api/security-scan", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        rootPath: rootPath,
        excluded: getExcludedProjects(),
        scanner: scanner,
        targetBranch: targetBranch,
      }),
    });

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;

      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() || "";

      for (const line of lines) {
        if (!line.trim()) continue;

        // SCAN_INIT:count:scanner
        if (line.startsWith("SCAN_INIT:")) {
          const parts = line.split(":");
          totalRepos = parseInt(parts[1]);
          const scannerType = parts[2] || "owasp";
          progressText.textContent = `Scanning... 0/${totalRepos} (${scannerType.toUpperCase()}, parallel)`;
          progressPercent.textContent = "0%";
          progressBar.style.width = "0%";
          progressEta.textContent = "Estimated: calculating...";
          continue;
        }

        // SCAN_PROGRESS:current:total:eta
        if (line.startsWith("SCAN_PROGRESS:")) {
          const parts = line.split(":");
          scannedRepos = parseInt(parts[1]);
          totalRepos = parseInt(parts[2]);
          const serverEta = parts[3] ? parseFloat(parts[3]) : null;
          const percent = Math.round((scannedRepos / totalRepos) * 100);

          progressText.textContent = `Scanning... ${scannedRepos}/${totalRepos}`;
          progressPercent.textContent = `${percent}%`;
          progressBar.style.width = `${percent}%`;
          progressBar.setAttribute("aria-valuenow", percent.toString());

          // Use server-provided ETA if available, otherwise calculate
          if (serverEta !== null && serverEta > 0) {
            progressEta.textContent = `Est. remaining: ${formatDuration(
              serverEta
            )}`;
          } else if (scannedRepos > 0) {
            const elapsed = (Date.now() - scanStartTime) / 1000;
            const avgTimePerRepo = elapsed / scannedRepos;
            const remaining = (totalRepos - scannedRepos) * avgTimePerRepo;
            progressEta.textContent = `Est. remaining: ${formatDuration(
              remaining
            )}`;
          }
          continue;
        }

        // REPO_START:name
        if (line.startsWith("REPO_START:")) {
          const repoName = line.split(":")[1];
          repoList.innerHTML += `<div id="security-repo-${repoName.replace(
            /[^a-zA-Z0-9]/g,
            "_"
          )}" style="padding: 4px 0; display: flex; align-items: center; gap: 8px;">
                  <span class="spinner" style="width: 12px; height: 12px; border-width: 2px;"></span>
                  <span style="color: #89b4fa;">${repoName}</span>
                </div>`;
          repoList.scrollTop = repoList.scrollHeight;
          continue;
        }

        // REPO_RESULT:{json}
        if (line.startsWith("REPO_RESULT:")) {
          try {
            const result = JSON.parse(line.substring(12));
            securityScanResults.push(result);

            // Update repo status in list
            const repoEl = document.getElementById(
              `security-repo-${result.repoName.replace(/[^a-zA-Z0-9]/g, "_")}`
            );
            if (repoEl) {
              const cveCount = result.findings ? result.findings.length : 0;
              const statusColor = result.error
                ? "#f38ba8"
                : cveCount > 0
                ? "#fab387"
                : "#a6e3a1";
              const statusText = result.error
                ? "✗ Skipped"
                : cveCount > 0
                ? `⚠ ${cveCount} CVEs`
                : "✓ Clean";
              repoEl.innerHTML = `<span style="color: ${statusColor};">${statusText}</span> <span style="color: #9ca0b0;">${result.repoName}</span>`;
            }
          } catch (e) {
            console.error("Failed to parse repo result:", e);
          }
          continue;
        }

        // REPO_DONE:name
        if (line.startsWith("REPO_DONE:")) {
          continue;
        }

        // SCAN_SUMMARY:critical:high:medium:low
        if (line.startsWith("SCAN_SUMMARY:")) {
          const parts = line.split(":");
          summaryStats = {
            critical: parseInt(parts[1]) || 0,
            high: parseInt(parts[2]) || 0,
            medium: parseInt(parts[3]) || 0,
            low: parseInt(parts[4]) || 0,
          };
          continue;
        }

        // SCAN_COMPLETE
        if (line.startsWith("SCAN_COMPLETE")) {
          progressPercent.textContent = "100%";
          progressBar.style.width = "100%";
          progressBar.setAttribute("aria-valuenow", "100");
          progressEta.textContent = "Complete!";
          continue;
        }
      }
    }

    // Display results
    displaySecurityResults();
    displaySecuritySummary(summaryStats);

    showToast(
      "Scan complete",
      `${securityScanResults.length} repositories scanned.`,
      "success",
      3000
    );
  } catch (e) {
    showToast("Error", e.message, "error");
    resultsDiv.innerHTML = `<div style="color: #f38ba8; grid-column: 1 / -1; text-align: center; padding: 40px;">Error: ${e.message}</div>`;
  } finally {
    btn.disabled = false;
    btn.innerHTML = "🔍 Scan for Vulnerabilities";
    exportBtn.disabled = securityScanResults.length === 0;
    isProcessRunning = false;
    setTimeout(() => progressDiv.classList.add("hidden"), 2000);
  }
}

// Display security scan results
function displaySecurityResults() {
  const resultsDiv = document.getElementById("security-results");

  if (securityScanResults.length === 0) {
    resultsDiv.innerHTML =
      '<div style="color: #9ca0b0; grid-column: 1 / -1; text-align: center; padding: 40px;">No repositories found to scan.</div>';
    return;
  }

  // Sort by CVE count (most vulnerabilities first)
  const sortedResults = [...securityScanResults].sort((a, b) => {
    const aCount = a.findings ? a.findings.length : 0;
    const bCount = b.findings ? b.findings.length : 0;
    return bCount - aCount;
  });

  let html = "";
  for (const result of sortedResults) {
    const cveCount = result.findings ? result.findings.length : 0;
    const hasError = !!result.error;

    let cardColor = "#a6e3a1"; // green for clean
    if (hasError) cardColor = "#f38ba8";
    else if (cveCount > 10) cardColor = "#f38ba8";
    else if (cveCount > 0) cardColor = "#fab387";

    // Project type badge
    const projectType = result.projectType || "unknown";
    let projectBadge = "";
    switch (projectType) {
      case "maven":
        projectBadge =
          '<span style="background: #fab38722; color: #fab387; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">☕ Maven</span>';
        break;
      case "npm":
        projectBadge =
          '<span style="background: #f38ba822; color: #f38ba8; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">📦 npm</span>';
        break;
      case "yarn":
        projectBadge =
          '<span style="background: #89b4fa22; color: #89b4fa; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">🧶 yarn</span>';
        break;
      case "pnpm":
        projectBadge =
          '<span style="background: #f9e2af22; color: #f9e2af; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">⚡ pnpm</span>';
        break;
      case "go":
        projectBadge =
          '<span style="background: #00ADD822; color: #00ADD8; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">🐹 Go</span>';
        break;
      case "python":
        projectBadge =
          '<span style="background: #3776AB22; color: #3776AB; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">🐍 Python</span>';
        break;
      case "php":
        projectBadge =
          '<span style="background: #8892BF22; color: #8892BF; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">🐘 PHP</span>';
        break;
      case "trivy":
        projectBadge =
          '<span style="background: #a6e3a122; color: #a6e3a1; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">🐳 Trivy</span>';
        break;
    }

    // Branch badge (show if available)
    const branchBadge = result.scannedBranch
      ? `<span style="background: #cba6f722; color: #cba6f7; padding: 2px 6px; border-radius: 4px; font-size: 0.7em; margin-left: 8px;">🔀 ${result.scannedBranch}</span>`
      : "";

    html += `<div class="card" style="border-left: 4px solid ${cardColor};">
            <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 10px;">
              <h4 style="margin: 0; color: ${cardColor};">📁 ${
      result.repoName
    }${projectBadge}${branchBadge}</h4>
              <div style="display: flex; align-items: center; gap: 10px;">
                <span style="color: #9ca0b0; font-size: 0.85em;">${
                  result.duration ? result.duration.toFixed(1) + "s" : ""
                }</span>
                <button onclick="exportSingleRepoSecurityPdf('${result.repoName.replace(
                  /'/g,
                  "\\'"
                )}')" class="btn btn-secondary" style="padding: 4px 8px; font-size: 0.75em;" title="Export PDF for this repo">📄</button>
              </div>
            </div>`;

    if (hasError) {
      html += `<div style="color: #f38ba8; padding: 10px; background: #f38ba822; border-radius: 4px;">
              <strong>Error:</strong> ${result.error}
            </div>`;
    } else if (cveCount === 0) {
      html += `<div style="color: #a6e3a1; padding: 10px; background: #a6e3a122; border-radius: 4px;">
              ✓ No vulnerabilities found
            </div>`;
    } else {
      // Group findings by severity
      const bySeverity = {
        CRITICAL: [],
        HIGH: [],
        MEDIUM: [],
        LOW: [],
        UNKNOWN: [],
      };
      for (const f of result.findings) {
        const sev = (f.severity || "UNKNOWN").toUpperCase();
        if (!bySeverity[sev]) bySeverity[sev] = [];
        bySeverity[sev].push(f);
      }

      html += `<div style="margin-bottom: 10px;">
              <span style="background: #f38ba822; color: #f38ba8; padding: 2px 8px; border-radius: 4px; font-size: 0.8em; margin-right: 5px;">Critical: ${bySeverity.CRITICAL.length}</span>
              <span style="background: #fab38722; color: #fab387; padding: 2px 8px; border-radius: 4px; font-size: 0.8em; margin-right: 5px;">High: ${bySeverity.HIGH.length}</span>
              <span style="background: #f9e2af22; color: #f9e2af; padding: 2px 8px; border-radius: 4px; font-size: 0.8em; margin-right: 5px;">Medium: ${bySeverity.MEDIUM.length}</span>
              <span style="background: #a6adc822; color: #a6adc8; padding: 2px 8px; border-radius: 4px; font-size: 0.8em;">Low: ${bySeverity.LOW.length}</span>
            </div>`;

      html += `<div style="max-height: 300px; overflow-y: auto;">`;

      // Show findings sorted by severity
      const severityOrder = ["CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"];
      for (const sev of severityOrder) {
        for (const f of bySeverity[sev]) {
          html += `<div style="padding: 8px; margin-bottom: 8px; background: var(--input-bg); border-radius: 4px; border-left: 3px solid ${getSeverityColor(
            f.severity
          )};">
                  <div style="display: flex; justify-content: space-between; align-items: center; margin-bottom: 4px;">
                    <a href="https://nvd.nist.gov/vuln/detail/${
                      f.cve
                    }" target="_blank" style="color: #89b4fa; text-decoration: none; font-weight: bold;">${
            f.cve
          }</a>
                    ${getSeverityBadge(f.severity)}
                  </div>
                  <div style="font-size: 0.85em; color: #cdd6f4;">${f.package}${
            f.version ? " @ " + f.version : ""
          }</div>
                  ${
                    f.fixedIn
                      ? `<div style="font-size: 0.8em; color: #a6e3a1;">Fixed in: ${f.fixedIn}</div>`
                      : ""
                  }
                  ${
                    f.description
                      ? `<div style="font-size: 0.8em; color: #9ca0b0; margin-top: 4px;">${f.description.substring(
                          0,
                          150
                        )}${f.description.length > 150 ? "..." : ""}</div>`
                      : ""
                  }
                </div>`;
        }
      }
      html += `</div>`;
    }

    html += `</div>`;
  }

  resultsDiv.innerHTML = html;
}

// Display security summary
function displaySecuritySummary(stats) {
  const summaryDiv = document.getElementById("security-summary");
  summaryDiv.classList.remove("hidden");

  const totalCves =
    (stats.critical || 0) +
    (stats.high || 0) +
    (stats.medium || 0) +
    (stats.low || 0);

  document.getElementById("security-total-repos").textContent =
    securityScanResults.length;
  document.getElementById("security-total-cves").textContent = totalCves;
  document.getElementById("security-critical").textContent =
    stats.critical || 0;
  document.getElementById("security-high").textContent = stats.high || 0;
  document.getElementById("security-medium").textContent = stats.medium || 0;
  document.getElementById("security-low").textContent = stats.low || 0;
}

// Export security report as PDF
async function exportSecurityPdf() {
  if (securityScanResults.length === 0) {
    showToast("No data", "Please run a scan first.", "warning");
    return;
  }

  showToast("Export", "Preparing PDF export...", "info", 2000);

  // Create printable HTML
  let html = `<!DOCTYPE html>
<html>
<head>
  <title>Security Scan Report</title>
  <style>
    body { font-family: Arial, sans-serif; padding: 20px; }
    h1 { color: #1e1e2e; border-bottom: 2px solid #89b4fa; padding-bottom: 10px; }
    h2 { color: #313244; margin-top: 30px; }
    .summary { display: flex; gap: 20px; margin: 20px 0; flex-wrap: wrap; }
    .summary-box { padding: 15px 25px; border-radius: 8px; text-align: center; }
    .critical { background: #f38ba822; border: 1px solid #f38ba8; }
    .high { background: #fab38722; border: 1px solid #fab387; }
    .medium { background: #f9e2af22; border: 1px solid #f9e2af; }
    .low { background: #a6adc822; border: 1px solid #a6adc8; }
    .repo { margin: 20px 0; padding: 15px; border: 1px solid #ddd; border-radius: 8px; }
    .repo h3 { margin: 0 0 10px 0; }
    .cve { padding: 8px; margin: 5px 0; background: #f5f5f5; border-radius: 4px; }
    .cve-id { font-weight: bold; color: #1e66f5; }
    table { width: 100%; border-collapse: collapse; margin: 10px 0; }
    th, td { padding: 8px; text-align: left; border-bottom: 1px solid #ddd; }
    th { background: #f5f5f5; }
    @media print { .no-print { display: none; } }
  </style>
</head>
<body>
  <h1>🛡️ Security Scan Report</h1>
  <p>Generated: ${new Date().toLocaleString()}</p>
  <p>Scanner: ${document
    .getElementById("security-scanner-select")
    .value.toUpperCase()}</p>

  <div class="summary">
    <div class="summary-box"><strong>${
      securityScanResults.length
    }</strong><br>Repos Scanned</div>
    <div class="summary-box critical"><strong>${
      document.getElementById("security-critical").textContent
    }</strong><br>Critical</div>
    <div class="summary-box high"><strong>${
      document.getElementById("security-high").textContent
    }</strong><br>High</div>
    <div class="summary-box medium"><strong>${
      document.getElementById("security-medium").textContent
    }</strong><br>Medium</div>
    <div class="summary-box low"><strong>${
      document.getElementById("security-low").textContent
    }</strong><br>Low</div>
  </div>

  <h2>Repository Details</h2>`;

  for (const result of securityScanResults) {
    const cveCount = result.findings ? result.findings.length : 0;
    html += `<div class="repo">
            <h3>📁 ${result.repoName}</h3>`;

    if (result.error) {
      html += `<p style="color: #e64553;">Error: ${result.error}</p>`;
    } else if (cveCount === 0) {
      html += `<p style="color: #40a02b;">✓ No vulnerabilities found</p>`;
    } else {
      html += `<table>
              <tr><th>CVE</th><th>Severity</th><th>Package</th><th>Version</th><th>Fixed In</th></tr>`;
      for (const f of result.findings) {
        html += `<tr>
                <td class="cve-id">${f.cve}</td>
                <td>${f.severity}</td>
                <td>${f.package}</td>
                <td>${f.version || "-"}</td>
                <td>${f.fixedIn || "-"}</td>
              </tr>`;
      }
      html += `</table>`;
    }
    html += `</div>`;
  }

  html += `</body></html>`;

  // Use hidden iframe for printing (no popup window)
  try {
    let iframe = document.getElementById("print-iframe");
    if (!iframe) {
      iframe = document.createElement("iframe");
      iframe.id = "print-iframe";
      iframe.style.position = "absolute";
      iframe.style.left = "-9999px";
      iframe.style.width = "0";
      iframe.style.height = "0";
      document.body.appendChild(iframe);
    }

    iframe.contentDocument.open();
    iframe.contentDocument.write(html);
    iframe.contentDocument.close();

    setTimeout(() => {
      iframe.contentWindow.focus();
      iframe.contentWindow.print();
    }, 300);
  } catch (e) {
    console.error("PDF export failed:", e);
    showToast("Error", "PDF export failed: " + e.message, "error");
  }
}

// Export single repository security report as PDF
function exportSingleRepoSecurityPdf(repoName) {
  const result = securityScanResults.find((r) => r.repoName === repoName);
  if (!result) {
    showToast("Error", "Repository not found.", "error");
    return;
  }

  showToast("Export", `Preparing PDF export for ${repoName}...`, "info", 2000);

  const cveCount = result.findings ? result.findings.length : 0;
  let criticalCount = 0,
    highCount = 0,
    mediumCount = 0,
    lowCount = 0;

  if (result.findings) {
    for (const f of result.findings) {
      const sev = (f.severity || "UNKNOWN").toUpperCase();
      if (sev === "CRITICAL") criticalCount++;
      else if (sev === "HIGH") highCount++;
      else if (sev === "MEDIUM") mediumCount++;
      else if (sev === "LOW") lowCount++;
    }
  }

  // Create printable HTML for single repo
  let html = `<!DOCTYPE html>
<html>
<head>
  <title>Security Report - ${repoName}</title>
  <style>
    body { font-family: Arial, sans-serif; padding: 20px; }
    h1 { color: #1e1e2e; border-bottom: 2px solid #89b4fa; padding-bottom: 10px; }
    h2 { color: #313244; margin-top: 30px; }
    .summary { display: flex; gap: 20px; margin: 20px 0; flex-wrap: wrap; }
    .summary-box { padding: 15px 25px; border-radius: 8px; text-align: center; }
    .critical { background: #f38ba822; border: 1px solid #f38ba8; }
    .high { background: #fab38722; border: 1px solid #fab387; }
    .medium { background: #f9e2af22; border: 1px solid #f9e2af; }
    .low { background: #a6adc822; border: 1px solid #a6adc8; }
    .clean { background: #a6e3a122; border: 1px solid #a6e3a1; }
    .cve { padding: 8px; margin: 5px 0; background: #f5f5f5; border-radius: 4px; }
    .cve-id { font-weight: bold; color: #1e66f5; }
    table { width: 100%; border-collapse: collapse; margin: 10px 0; }
    th, td { padding: 8px; text-align: left; border-bottom: 1px solid #ddd; }
    th { background: #f5f5f5; }
    @media print { .no-print { display: none; } }
  </style>
</head>
<body>
  <h1>🛡️ Security Report: ${repoName}</h1>
  <p>Generated: ${new Date().toLocaleString()}</p>
  <p>Scanner: ${document
    .getElementById("security-scanner-select")
    .value.toUpperCase()}</p>
  ${
    result.duration
      ? `<p>Scan Duration: ${result.duration.toFixed(1)}s</p>`
      : ""
  }

  <div class="summary">
    <div class="summary-box"><strong>${cveCount}</strong><br>Total CVEs</div>
    <div class="summary-box critical"><strong>${criticalCount}</strong><br>Critical</div>
    <div class="summary-box high"><strong>${highCount}</strong><br>High</div>
    <div class="summary-box medium"><strong>${mediumCount}</strong><br>Medium</div>
    <div class="summary-box low"><strong>${lowCount}</strong><br>Low</div>
  </div>`;

  if (result.error) {
    html += `<h2>Error</h2>
          <p style="color: #e64553; background: #f5f5f5; padding: 15px; border-radius: 8px;">${result.error}</p>`;
  } else if (cveCount === 0) {
    html += `<div class="summary-box clean" style="margin-top: 20px; text-align: center;">
            <strong style="color: #40a02b; font-size: 1.2em;">✓ No vulnerabilities found</strong>
          </div>`;
  } else {
    html += `<h2>Vulnerabilities</h2>
          <table>
            <tr><th>CVE</th><th>Severity</th><th>Package</th><th>Version</th><th>Fixed In</th><th>Description</th></tr>`;

    // Sort by severity
    const severityOrder = {
      CRITICAL: 0,
      HIGH: 1,
      MEDIUM: 2,
      LOW: 3,
      UNKNOWN: 4,
    };
    const sortedFindings = [...result.findings].sort((a, b) => {
      const sevA = (a.severity || "UNKNOWN").toUpperCase();
      const sevB = (b.severity || "UNKNOWN").toUpperCase();
      return (severityOrder[sevA] || 4) - (severityOrder[sevB] || 4);
    });

    for (const f of sortedFindings) {
      html += `<tr>
              <td class="cve-id">${f.cve}</td>
              <td>${f.severity}</td>
              <td>${f.package}</td>
              <td>${f.version || "-"}</td>
              <td>${f.fixedIn || "-"}</td>
              <td style="font-size: 0.85em;">${
                f.description
                  ? f.description.substring(0, 100) +
                    (f.description.length > 100 ? "..." : "")
                  : "-"
              }</td>
            </tr>`;
    }
    html += `</table>`;
  }

  html += `</body></html>`;

  // Use hidden iframe for printing (no popup window)
  try {
    let iframe = document.getElementById("print-iframe");
    if (!iframe) {
      iframe = document.createElement("iframe");
      iframe.id = "print-iframe";
      iframe.style.position = "absolute";
      iframe.style.left = "-9999px";
      iframe.style.width = "0";
      iframe.style.height = "0";
      document.body.appendChild(iframe);
    }

    iframe.contentDocument.open();
    iframe.contentDocument.write(html);
    iframe.contentDocument.close();

    setTimeout(() => {
      iframe.contentWindow.focus();
      iframe.contentWindow.print();
    }, 300);
  } catch (e) {
    console.error("Single repo PDF export failed:", e);
    showToast("Fehler", "PDF-Export fehlgeschlagen: " + e.message, "error");
  }
}
