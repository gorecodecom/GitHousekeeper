# GitHousekeeper

GitHousekeeper is a powerful tool designed to automate maintenance tasks and mass-refactoring across multiple Git repositories. It provides a user-friendly Web GUI to orchestrate updates, manage versions, and perform project-wide replacements efficiently.

## Features

- **Multi-Repository Scanning**: Automatically finds all Git repositories within a specified root directory.
- **Web Interface**: Intuitive local web interface for configuration and monitoring.
- **Automated Versioning**:
  - Detects the latest Git tag.
  - Updates the project version in `pom.xml`.
  - Supports **Major**, **Minor**, and **Patch** version bumping strategies.
- **Mass Search & Replace**:
  - **POM Replacements**: Targeted fuzzy search and replace within `pom.xml` files.
  - **Project-Wide Replacements**: Fuzzy search and replace across all project files (excluding `.git`, `target`, etc.).
- **Maven Integration**:
  - Updates `<parent>` versions in `pom.xml`.
  - Automatically runs `mvn clean install -DskipTests` to verify changes after project-wide replacements.
- **Git Automation**:
  - Automatically manages a `housekeeping` branch.
  - Resets the branch if it is stale (older than 1 month).
  - Commits changes automatically with descriptive messages.

## Prerequisites

- **Go**: To build and run the application.
- **Git**: Must be installed and available in the system PATH.
- **Maven**: Required for project verification steps.

## Installation & Usage

1. **Clone the repository**:
   ```bash
   git clone https://github.com/gorecodecom/GitHousekeeper.git
   cd GitHousekeeper
   ```

2. **Run the application**:
   ```bash
   go run main.go
   ```
   The application will start a local web server and attempt to open your default browser to `http://localhost:8080`.

3. **Configure the Housekeeping Run**:
   - **Root Path**: Enter the directory containing your Git repositories.
   - **Excluded Folders**: Specify folders to ignore (e.g., `node_modules`, `dist`).
   - **Parent Version**: (Optional) Target version for the Maven parent POM.
   - **Version Bump**: Choose how to increment the version (Patch, Minor, Major).

4. **Define Replacements**:
   - Use the **POM Replacements** tab for specific changes in `pom.xml`.
   - Use the **Project Replacements** tab for global changes.

5. **Start**:
   - Click **Start** to begin the process.
   - Monitor the progress in the **Report** tab.

## How it Works

For each repository found:
1. Checks out or creates a `housekeeping` branch.
2. Fetches the latest tags and updates.
3. Updates `pom.xml` version based on the latest Git tag and selected strategy.
4. Performs defined text replacements.
5. If project files were modified, runs a Maven build to ensure integrity.
6. Commits changes if successful.

## License

[MIT](LICENSE)
