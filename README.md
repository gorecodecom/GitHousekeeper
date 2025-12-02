# GitHousekeeper

GitHousekeeper is a powerful tool designed to automate maintenance tasks and mass-refactoring across multiple Git repositories. It provides a user-friendly Web GUI to orchestrate updates, manage versions, and perform project-wide replacements efficiently.

## Features

- **Multi-Repository Scanning**: Automatically finds all Git repositories within a specified root directory.
- **Modern Web Interface**:
  - **Live-Logging**: Real-time feedback during the build and update process.
  - **Settings Persistence**: Automatically remembers your paths and configuration between sessions.
  - **Folder Picker**: Helper buttons to easily select folders (with clipboard support).
- **Automated Versioning**:
  - Detects the latest Git tag.
  - Updates the project version in `pom.xml`.
  - Supports **Major**, **Minor**, and **Patch** version bumping strategies.
- **Mass Search & Replace**:
  - **POM Replacements**: Targeted fuzzy search and replace within `pom.xml` files.
  - **Project-Wide Replacements**: Fuzzy search and replace across all project files (excluding `.git`, `target`, etc.).
  - **Smart Indentation**: Automatically detects and preserves the indentation of replaced blocks, ensuring clean XML/code formatting.
- **Maven Integration**:
  - Updates `<parent>` versions in `pom.xml`.
  - **Optimized Build**: Runs `mvn clean install` and checks for deprecation warnings in a single efficient pass.
  - **Deprecation Reporting**: Captures and displays the top 100 deprecation warnings per repository in a dedicated view.
- **Spring Boot Insights**:
  - **Version Dashboard**: View currently available Spring Boot versions (Major/Minor) fetched live from Maven Central.
  - **Migration Guides**: Direct links to official migration guides for major version upgrades.
  - **Project Scanning**: Scans local repositories to identify their current Spring Boot parent version.
- **Reporting & Export**:
  - Detailed execution log.
  - **PDF Export**: Export the general log or the deprecation report as a PDF file.
- **Git Automation**:
  - **Flexible Branching Strategy**:
    - **Housekeeping**: Default mode. Manages a `housekeeping` branch (resets if stale > 1 month).
    - **Custom Branch**: Work on a specific feature branch (e.g., `feature/upgrade-v2`).
    - **Direct Master**: Option to apply changes directly to the `master` branch.
  - Automatically commits changes with descriptive messages.

## Prerequisites

To run the pre-built executable:
- **Git**: Must be installed and available in the system PATH.
- **Maven**: Required for project verification steps (`mvn` command).

To build from source:
- **Go**: Version 1.16 or higher.

## Installation & Usage

### Option A: Run Pre-built Executable (Windows)
1. Simply download or build `GitHousekeeper.exe`.
2. Double-click `GitHousekeeper.exe`.
3. The application will start and open your browser at `http://localhost:8080`.

### Option B: Build from Source
1. **Clone the repository**:
   ```bash
   git clone https://github.com/gorecodecom/GitHousekeeper.git
   cd GitHousekeeper
   ```

2. **Build the application**:
   ```bash
   go build -o GitHousekeeper.exe main.go
   ```
   *Note: The HTML/CSS assets are embedded directly into the executable. You only need the `.exe` file to run the app.*

3. **Run**:
   ```bash
   ./GitHousekeeper.exe
   ```

### Development Mode

If you want to modify the frontend (HTML/CSS/JS) without rebuilding the Go application:

1. Ensure the `assets` folder is present in the same directory as the executable.
2. Run the application. It will detect the `assets` folder and serve files from disk instead of the embedded filesystem.
3. Refresh your browser to see changes instantly.

## Workflow

1. **Configure**:
   - **Root Path**: Select the directory containing your Git repositories.
   - **Included Projects**: Dynamically select which subfolders (repositories) to include or exclude via checkboxes.
   - **Settings**: Choose version bump strategy and whether to run a full Maven build.

2. **Define Replacements**:
   - Use the **POM Replacements** tab for specific changes in `pom.xml`.
   - Use the **Project Replacements** tab for global changes.

3. **Start**:
   - Click **Start** to begin.
   - Follow the **Live Log** in the Report tab.
   - Review **Deprecation Warnings** in the side panel.
   - Export reports to PDF if needed.

## License

[MIT](LICENSE)
