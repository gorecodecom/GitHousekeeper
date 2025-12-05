# Changelog

All notable changes to GitHousekeeper will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.4.0] - 2025-12-05

### Added

- **📦 Full-Stack Security Scanner - Node.js Support**
  - Extended CVE vulnerability scanning to support Node.js/Frontend projects
  - npm audit integration for projects with `package-lock.json`
  - yarn audit integration for projects with `yarn.lock`
  - pnpm audit integration for projects with `pnpm-lock.yaml`
  - Automatic parsing of all package manager audit formats

- **🔀 Branch Selection for Security Scans**
  - New "Target Branch" dropdown in the Security Scanner
  - Scan all repositories on a specific branch (e.g., `main`, `develop`, `release/1.0`)
  - Automatic branch switching during scan with safe stash/restore of uncommitted changes
  - Shows "Current branch" as default option
  - Branch list populated from all available branches across all repositories
  - Default branches (main/master) shown first with ⭐ icon

- **🧶 Yarn Berry (v2/v3/v4) Support**
  - Full support for Yarn Modern (Berry) including versions 2.x, 3.x, and 4.x
  - Automatic detection of Yarn version via `packageManager` field in `package.json`
  - Corepack integration for projects using managed Yarn versions
  - New `parseYarnBerryAuditOutput()` parser for Yarn Berry's unique NDJSON format
  - Correctly parses Yarn Berry's `yarn npm audit --json` output format:
    ```json
    {"value":"next","children":{"ID":1111229,"Issue":"...","Severity":"critical",...}}
    ```
  - Separate `parseYarnClassicAuditOutput()` for Yarn Classic (v1) format
  - Automatic fallback: detects global vs project-local Yarn version

- **🔄 Auto-detect Project Type**
  - New "Auto-detect" scanner mode (recommended default)
  - Automatically detects project type based on lock files:
    - `pom.xml` → Maven (OWASP Dependency-Check)
    - `package-lock.json` → npm audit
    - `yarn.lock` → yarn audit (Classic or Berry, auto-detected)
    - `pnpm-lock.yaml` → pnpm audit
  - Mixed repositories: scans using detected package manager

- **🎨 UI Enhancements**
  - Project type badges in scan results (☕ Maven, 📦 npm, 🧶 yarn, ⚡ pnpm, 🐳 Trivy)
  - Scanner selection dropdown with clear icons and descriptions
  - Package manager availability check in UI
  - Updated scanner descriptions for all options

### Changed

- Scanner dropdown now defaults to "Auto-detect (recommended)"
- Trivy scanner now supports both Maven and Node.js projects
- Updated description text to reflect multi-language support
- `detectYarnVersion()` now reads `packageManager` field from `package.json` first
- Yarn commands use `corepack yarn` when `packageManager` is defined
- **Yarn Berry now uses text output instead of JSON**: More reliable scanning since `yarn npm audit --json` often fails with HTTP 500 errors from GitHub Advisory API

### Fixed

- **Yarn Berry audit parsing**: Fixed issue where Yarn v2/v3/v4 projects showed "No vulnerabilities found" despite having CVEs
- **Corepack compatibility**: Projects with `packageManager: "yarn@4.0.2"` now correctly use corepack to run the project-specific Yarn version
- **CVE-2025-66478 detection**: Critical Next.js RCE vulnerability now correctly detected in Yarn Berry projects
- **Yarn Berry JSON API failure**: Added `parseYarnBerryTextOutput()` parser that parses human-readable text output when JSON fails
- **Branch-specific scanning**: Fixed issue where scanning different branches showed same results due to Yarn Berry JSON API returning HTTP 500 errors

## [2.3.0] - 2025-12-04

### Added

- **🛡️ New Security Tab with CVE Vulnerability Scanner**
  - Scan all Maven projects for known CVE vulnerabilities
  - Support for OWASP Dependency-Check Maven Plugin (12.1.0)
  - Optional Trivy scanner integration (auto-detect available scanners)
  - Parallel scanning with worker pool (4 concurrent scans)
  - Live progress bar with ETA and percentage display
  - Per-repository status cards showing scan progress
  - Severity-based CVE grouping (Critical, High, Medium, Low)
  - Direct links to NVD for CVE details
  - **Per-Repository PDF Export**: Export security report for individual repos
  - Full report PDF export for all scanned repositories
  - **Re-check Button**: Verify Trivy availability after installation without page reload

- **🔧 New Maintenance Tab**
  - Branch overview showing all local branches per repository
  - Tracking status with ahead/behind counts for each branch
  - One-click "Sync All Tracked Branches" to fetch and pull all repos
  - Live progress bar and detailed sync log
  - Accessible with ARIA labels, roles, and keyboard navigation

- **🔀 Auto-detect Default Branch**
  - Automatically detects `main` or `master` as default branch per repository
  - Falls back gracefully: symbolic-ref → local main → remote main → master
  - No more hardcoded "master" - works with modern Git workflows

- **⚡ Performance Optimizations**
  - Server-side caching for Spring Boot versions (5 minute TTL)
  - Server-side caching for OpenRewrite versions (10 minute TTL)
  - Frontend caching to prevent redundant API calls
  - Faster page load times for Migration Assistant tab

### Fixed

- **🔄 Automatic Retry for Migration Assistant**
  - Added retry logic for Maven analysis (1 retry on failure)
  - Helps with intermittent Maven dependency caching issues
  - Reduces failed scans that would succeed on manual retry

### Changed

- Sidebar reorganized: Security and Maintenance tabs now appear for quick access

## [2.2.1] - 2025-12-04

### Added

- **📊 Improved Progress Display for Migration Assistant**
  - Live repository status cards showing each project during parallel analysis
  - Animated progress bar per repository with "analyzing..." indicator
  - Real-time updates as each repository completes (success/failure with duration)
  - Responsive grid layout (up to 3 repos per row)
  - Visual feedback with pulsing animation while analysis is running

### Fixed

- Progress bar now shows meaningful intermediate states instead of jumping from 0% to 100%
- Added proper spacing between "Target Spring Boot Version" section and progress display

## [2.2.0] - 2025-12-04

### Added

- **🔔 Error Handling & User Feedback**
  - Toast notification system for success, error, and warning messages
  - Connection status banner with offline detection and server health monitoring
  - Server health endpoint (`/api/health`) for connectivity checks
  - Warning dialog when closing browser tab with running process
  - Automatic reconnection detection with user notification

- **♿ Accessibility (a11y)**
  - Full keyboard navigation for sidebar menu (Arrow keys, Enter, Space)
  - Skip-to-content link for screen reader users
  - ARIA roles and labels throughout the UI (menubar, menuitem, navigation)
  - Focus-visible styles for all interactive elements
  - Improved color contrast for WCAG AA compliance
  - Semantic HTML with proper landmark regions

### Technical

- Added 20 unit tests covering replacement scope routing and health endpoint
- Test coverage for edge cases in unified replacements feature

## [2.1.0] - 2025-12-04

### Changed

- **Unified Replacements Tab** - Merged "POM Replacements" and "Project Replacements" into a single "Replacements" tab
  - New **Scope Selection** with radio buttons: "All Files", "Only pom.xml", or "Exclude pom.xml"
  - Cleaner UI with explanation card describing fuzzy matching and smart indentation
  - Simplified workflow - one place for all replacement operations

### Updated

- Screenshots updated to reflect new unified Replacements tab
- README documentation updated for new replacement workflow

## [2.0.0] - 2025-12-03

### Added

- **📊 Dashboard & Analytics** (NEW TAB)

  - Repository Health Score (0-100) with penalty system for outdated frameworks, TODOs, and JUnit 4 usage
  - Total Repositories count with active project monitoring
  - Technical Debt tracking (TODO/FIXME count across all files)
  - **Top Dependencies Chart** - Bar chart showing most used dependencies (excluding standard Spring Boot deps)
  - **Spring Boot Versions Chart** - Visual distribution of Spring Boot versions across projects
  - **Repository Details Table** - Health Score, Spring Boot version, Java version, last commit date, TODO count per repo
  - Streaming data loading for responsive feedback during analysis
  - CSS Grid-based responsive chart layout
  - Empty state with onboarding flow for new users

- **📚 Framework Info** (NEW TAB)

  - Centralized reference for framework information
  - **Jakarta EE Overview** - Namespace change documentation (javax._ → jakarta._)
  - **Quarkus Information** - Version comparison and migration paths
  - **Java SE Support Matrix** - LTS versions, release dates, and support timelines (Java 8 → Java 25)

- **🚀 Migration Assistant** (Redesigned)

  - **Migration Type Selection** - Radio button UI for choosing migration type
  - **Spring Boot Upgrade** - Upgrade between Spring Boot versions (2.x → 3.x → 3.5)
  - **Java Version Upgrade** - Java 8 → 17 → 21 migration recipes
  - **Jakarta EE Migration** - Dedicated javax._ to jakarta._ migration
  - **Quarkus Migration** - Migration path to Quarkus 2.x

- **🎨 UI/UX Improvements**

  - **Redesigned Navigation** - Icons added to all sidebar menu items
  - **Separate CSS File** - Extracted styles to `styles.css` for better maintainability
  - **Remove Row Button** - Delete individual replacement rows in POM/Project Replacements
  - **Auto-resizing Textareas** - Textareas grow automatically with content
  - **Reset Button** - Clear all saved settings and return to defaults in Project Setup
  - **Card-based Layout** - Consistent card styling across all tabs
  - **Status Badges** - Color-coded badges for health status (Good/Warning/Critical)

- **🔧 Technical Improvements**
  - Root Path input moved to Dashboard for centralized management
  - Improved folder picker integration
  - Better error handling and loading states

### Changed

- **Navigation Structure** - Dashboard is now the default landing page
- **Frameworks Tab** - Split into "Framework Info" (reference) and "Migration Assistant" (actions)
- **Project Setup** - Simplified, path management moved to Dashboard

### Fixed

- False positive deprecation count from Maven compiler [INFO] lines
- Git repository discovery in subfolders (removed .git from exclusion list)
- Branch Strategy label display ("Housekeeping" instead of "housekeeping")

## [1.0.0] - 2025-12-02

### Added

- **Spring Boot Migration Analysis** using OpenRewrite

  - Parallel processing of multiple projects with goroutines
  - Visual progress bar with percentage, ETA, and remaining time estimation
  - Smart summary parsing - categorizes changes instead of showing raw patch output
  - Support for Spring Boot 2.x → 3.x and 3.x → 3.5 migrations
  - Uses OpenRewrite Maven Plugin 6.24.0 with rewrite-spring 6.19.0
  - **OpenRewrite Version Monitoring**: Displays current vs. latest versions with update notifications

- **Spring Boot Version Dashboard**

  - Live version fetching from Maven Central
  - Grouped by Major.Minor branches
  - Shows 5 newest branches by default with expand option
  - Direct links to migration guides and release notes

- **Project Scanning**

  - Auto-detection of Spring Boot parent versions in local projects
  - Support for single repo or multi-repo root paths

- **Multi-Repository Management**

  - Auto-discovery of Git repositories
  - Checkbox-based include/exclude selection
  - Batch operations across multiple repos

- **Automated Versioning**

  - Git tag detection
  - Major/Minor/Patch version bumping
  - Parent version updates in pom.xml

- **Mass Search & Replace**

  - POM-specific replacements
  - Project-wide replacements
  - Smart indentation preservation

- **Maven Integration**

  - Optimized build with deprecation warning capture
  - Deprecation report (top 100 warnings per repo)

- **Git Automation**

  - Flexible branching strategies (housekeeping, custom, direct master)
  - Automatic commit with descriptive messages

- **Modern Web Interface**
  - Dark theme
  - Live logging with streaming output
  - Settings persistence (localStorage)
  - Native OS folder picker
  - PDF export for reports

### Technical

- Go 1.21+ with embedded assets
- Single executable deployment
- Development mode with hot-reload for frontend changes
