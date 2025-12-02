# Changelog

All notable changes to GitHousekeeper will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.0] - 2025-12-02

### Added

- **Spring Boot Migration Analysis** using OpenRewrite

  - Parallel processing of multiple projects with goroutines
  - Visual progress bar with percentage, ETA, and remaining time estimation
  - Smart summary parsing - categorizes changes instead of showing raw patch output
  - Support for Spring Boot 2.x → 3.x and 3.x → 3.5 migrations
  - Uses OpenRewrite Maven Plugin 6.24.0 with rewrite-spring 6.19.0

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
