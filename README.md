# GitHousekeeper

GitHousekeeper is a powerful tool designed to automate maintenance tasks and mass-refactoring across multiple Git repositories. It provides a user-friendly Web GUI to orchestrate updates, manage versions, and perform project-wide replacements efficiently.

## 📥 Download

**Current Version: 2.1.0**

Download the pre-built executable for your platform:

| Platform                  | Download                                                                                                                                    | Notes                |
| ------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | -------------------- |
| **Windows**               | [GitHousekeeper-windows-amd64.exe](https://github.com/gorecodecom/GitHousekeeper/releases/latest/download/GitHousekeeper-windows-amd64.exe) | 64-bit Windows 10/11 |
| **macOS (Intel)**         | [GitHousekeeper-darwin-amd64](https://github.com/gorecodecom/GitHousekeeper/releases/latest/download/GitHousekeeper-darwin-amd64)           | Intel-based Macs     |
| **macOS (Apple Silicon)** | [GitHousekeeper-darwin-arm64](https://github.com/gorecodecom/GitHousekeeper/releases/latest/download/GitHousekeeper-darwin-arm64)           | M1/M2/M3 Macs        |
| **Linux**                 | [GitHousekeeper-linux-amd64](https://github.com/gorecodecom/GitHousekeeper/releases/latest/download/GitHousekeeper-linux-amd64)             | 64-bit Linux         |

> 💡 **Tip**: On macOS/Linux, you may need to make the file executable: `chmod +x GitHousekeeper-*`

See [CHANGELOG.md](CHANGELOG.md) for release history and all versions on the [Releases page](https://github.com/gorecodecom/GitHousekeeper/releases).

## Features

### 🔍 Multi-Repository Management

- **Auto-Discovery**: Automatically finds all Git repositories within a specified root directory.
- **Selective Processing**: Include/exclude specific projects via checkbox selection.
- **Batch Operations**: Apply changes across dozens of repositories simultaneously.

### 🌐 Modern Web Interface

- **Live-Logging**: Real-time feedback during build and update processes.
- **Settings Persistence**: Automatically remembers your paths and configuration between sessions.
- **Native Folder Picker**: OS-native dialog to select folders easily.
- **Dark Theme**: Easy on the eyes for extended use.

### 🏷️ Automated Versioning

- Detects the latest Git tag per repository.
- Updates the project version in `pom.xml`.
- Supports **Major**, **Minor**, and **Patch** version bumping strategies.

### 🔄 Mass Search & Replace

- **Unified Replacements Tab**: Single interface with file scope selection.
- **Scope Options**: Choose between "All Files", "Only pom.xml", or "Exclude pom.xml".
- **Fuzzy Matching**: Smart search that handles whitespace and indentation differences.
- **Smart Indentation**: Automatically detects and preserves the indentation of replaced blocks, ensuring clean XML/code formatting.

### 🛠️ Maven Integration

- Updates `<parent>` versions in `pom.xml`.
- **Optimized Build**: Runs `mvn clean install` and checks for deprecation warnings in a single efficient pass.
- **Deprecation Reporting**: Captures and displays the top 100 deprecation warnings per repository in a dedicated view.

### 🍃 Spring Boot Insights

- **Version Dashboard**: View all available Spring Boot versions (grouped by Major.Minor) fetched live from Maven Central.
- **Migration Guides**: Direct links to official migration guides for major version upgrades.
- **Project Scanning**: Scans local repositories to identify their current Spring Boot parent version.
- **Expandable Version List**: Shows the 5 newest version branches by default, with option to show older versions.

### 🚀 Spring Boot Migration Analysis (OpenRewrite)

- **Parallel Processing**: Analyzes multiple projects simultaneously using Go routines for maximum speed.
- **Progress Tracking**: Visual progress bar with percentage, ETA, and estimated remaining time.
- **Smart Summary**: Categorizes proposed changes instead of showing raw patch output:
  - 🔄 **Annotation Updates** (e.g., `@RequestMapping` → `@GetMapping`)
  - 📦 **Import Changes**
  - 🛠️ **Code Modernization** (e.g., Pattern Matching, `String.formatted()`)
  - ⚙️ **Configuration Changes** (deprecated properties)
  - 🗑️ **Deprecated Code Removal** (e.g., unnecessary `@Autowired`)
- **Dry-Run Mode**: Analyzes projects without modifying any files.
- **Zero-Config**: Injects the OpenRewrite Maven plugin dynamically—no changes to your `pom.xml` required.
- **Version Monitoring**: Displays current vs. latest OpenRewrite versions with update notifications.
- **Latest Recipes**: Uses OpenRewrite Maven Plugin 6.24.0 with rewrite-spring 6.19.0 (supports Spring Boot 3.5).

### 📊 Reporting & Export

- Detailed execution log with color-coded output.
- **PDF Export**: Export the general log or the deprecation report as a PDF file.

### 🔀 Git Automation

- **Flexible Branching Strategy**:
  - **Housekeeping**: Default mode. Manages a `housekeeping` branch (resets if stale > 1 month).
  - **Custom Branch**: Work on a specific feature branch (e.g., `feature/upgrade-v2`).
  - **Direct Master**: Option to apply changes directly to the `master` branch.
- Automatically commits changes with descriptive messages.

## Prerequisites

To run the pre-built executable:

- **Git**: Must be installed and available in the system PATH.
- **Maven**: Required for project builds and OpenRewrite analysis (`mvn` command).
- **Java**: JDK 17+ recommended for Spring Boot 3.x projects.

To build from source:

- **Go**: Version 1.21 or higher.

## Installation & Usage

### Option A: Run Pre-built Executable

#### Windows

1. Download `GitHousekeeper-windows-amd64.exe` from the [Releases page](https://github.com/gorecodecom/GitHousekeeper/releases).
2. Double-click the `.exe` file.
3. Your browser will open automatically at `http://localhost:8080`.

#### macOS

1. Download the appropriate version for your Mac:

   - **Intel Mac**: `GitHousekeeper-darwin-amd64`
   - **Apple Silicon (M1/M2/M3)**: `GitHousekeeper-darwin-arm64`

2. Open Terminal and make the file executable:

   ```bash
   chmod +x ~/Downloads/GitHousekeeper-darwin-*
   ```

3. On first run, macOS may block the app. To allow it:

   - Right-click the file → **Open**, or
   - Go to **System Preferences → Security & Privacy → General** and click **Open Anyway**

4. Run the application:

   ```bash
   ~/Downloads/GitHousekeeper-darwin-arm64
   ```

5. Your browser will open automatically at `http://localhost:8080`.

#### Linux

1. Download `GitHousekeeper-linux-amd64` from the [Releases page](https://github.com/gorecodecom/GitHousekeeper/releases).

2. Make the file executable:

   ```bash
   chmod +x GitHousekeeper-linux-amd64
   ```

3. Run the application:

   ```bash
   ./GitHousekeeper-linux-amd64
   ```

4. Your browser will open automatically at `http://localhost:8080`.

   > **Note**: On Linux, the folder picker dialog requires `zenity` (GNOME/GTK) or `kdialog` (KDE) to be installed.

### Option B: Build from Source

#### Prerequisites

- **Go**: Version 1.21 or higher ([Download](https://go.dev/dl/))

#### Build Steps (All Platforms)

1. **Clone the repository**:

   ```bash
   git clone https://github.com/gorecodecom/GitHousekeeper.git
   cd GitHousekeeper
   ```

2. **Build for your current platform**:

   ```bash
   go build -o GitHousekeeper .
   ```

   Or build for a specific platform:

   ```bash
   # Windows
   GOOS=windows GOARCH=amd64 go build -o GitHousekeeper-windows-amd64.exe .

   # macOS Intel
   GOOS=darwin GOARCH=amd64 go build -o GitHousekeeper-darwin-amd64 .

   # macOS Apple Silicon
   GOOS=darwin GOARCH=arm64 go build -o GitHousekeeper-darwin-arm64 .

   # Linux
   GOOS=linux GOARCH=amd64 go build -o GitHousekeeper-linux-amd64 .
   ```

   > 💡 The HTML/CSS/JS assets are embedded directly into the executable. You only need the single binary file to run the app.

3. **Run**:
   ```bash
   ./GitHousekeeper
   ```

### Development Mode

If you want to modify the frontend (HTML/CSS/JS) without rebuilding the Go application:

1. Ensure the `assets` folder is present in the same directory as the executable.
2. Run the application. It will detect the `assets` folder and serve files from disk instead of the embedded filesystem.
3. Refresh your browser to see changes instantly.

## Workflow

### General Housekeeping

1. **Configure**:

   - **Root Path**: Select the directory containing your Git repositories.
   - **Included Projects**: Select which subfolders (repositories) to include or exclude.
   - **Settings**: Choose version bump strategy and whether to run a full Maven build.

2. **Define Replacements**:

   - Use the **Replacements** tab to define search/replace patterns.
   - Select the **Scope**: All Files, Only pom.xml, or Exclude pom.xml.

3. **Execute**:
   - Click **Start** to begin.
   - Follow the **Live Log** in the Report tab.
   - Review **Deprecation Warnings** in the side panel.
   - Export reports to PDF if needed.

### Spring Boot Migration Analysis

1. Navigate to the **Migration Assistant** tab.
2. Select a migration type (Spring Boot, Java, Jakarta EE, or Quarkus).
3. Choose a **Target Version** from the dropdown.
4. Click **Analyze** to start the OpenRewrite dry-run.
5. Review the categorized summary showing what changes would be made.
6. Apply changes manually or use OpenRewrite's `run` goal to apply them automatically.

## Screenshots

### Dashboard

![Dashboard](screenshots/01_Dashboard.png)
_Repository health overview with analytics and metrics_

### Project Setup

![Project Setup](screenshots/02_Project_Setup.png)
_Configure projects, branch strategy, and version settings_

### Replacements

![Replacements](screenshots/03_POM_Replacements.png)
_Unified search and replace with file scope selection (All Files / Only pom.xml / Exclude pom.xml)_

### Report

![Report](screenshots/04_Report.png)
_Live execution log with deprecation warnings_

### Framework Info

![Framework Info](screenshots/05_Framework_Info.png)
_Spring Boot versions, Java SE support matrix, and framework references_

### Migration Assistant

![Migration Assistant](screenshots/06_Migration_Assistant.png)
_OpenRewrite-powered migration analysis for Spring Boot, Java, and Jakarta EE_

## Contributing

Contributions are welcome! Please read our [Contributing Guide](CONTRIBUTING.md) for details on:

- Setting up the development environment
- Code style guidelines
- Submitting pull requests

## Security

Found a vulnerability? Please read our [Security Policy](SECURITY.md) for responsible disclosure guidelines.

## Author

**GoreCode**
GitHub: [@gorecodecom](https://github.com/gorecodecom)

## License

[MIT](LICENSE) © 2025 GoreCode

---

Made with ❤️ for developers who manage multiple Spring Boot microservices.
