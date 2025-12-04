# Contributing to GitHousekeeper

Thank you for your interest in contributing to GitHousekeeper! This document provides guidelines and instructions for contributing.

## 🚀 Getting Started

### Prerequisites

- **Go**: Version 1.21 or higher ([Download](https://go.dev/dl/))
- **Git**: For version control
- **Maven**: For testing OpenRewrite functionality
- **Java**: JDK 17+ for Spring Boot 3.x testing

### Development Setup

1. **Fork and clone the repository**:

   ```bash
   git clone https://github.com/YOUR_USERNAME/GitHousekeeper.git
   cd GitHousekeeper
   ```

2. **Run in development mode**:

   ```bash
   go run .
   ```

   The application will detect the local `assets` folder and serve files from disk, allowing you to modify HTML/CSS/JS without rebuilding.

3. **Build the binary**:

   ```bash
   go build -o GitHousekeeper .
   ```

### Project Structure

```
GitHousekeeper/
├── main.go              # Main application, HTTP handlers
├── internal/
│   └── logic/
│       ├── logic.go     # Core business logic
│       ├── logic_test.go # Unit tests
│       └── dashboard.go # Dashboard statistics
├── assets/
│   ├── index.html       # Main HTML page
│   ├── app.js           # Frontend JavaScript
│   └── styles.css       # Styling
├── tests/               # Test setup scripts
└── screenshots/         # Documentation images
```

## 📝 How to Contribute

### Reporting Bugs

1. Check [existing issues](https://github.com/gorecodecom/GitHousekeeper/issues) to avoid duplicates.
2. Create a new issue with:
   - Clear, descriptive title
   - Steps to reproduce
   - Expected vs. actual behavior
   - OS, Go version, and any relevant environment details

### Suggesting Features

1. Open an issue with the `enhancement` label.
2. Describe the feature and its use case.
3. If possible, outline a proposed implementation.

### Submitting Code Changes

1. **Create a branch**:

   ```bash
   git checkout -b feature/your-feature-name
   ```

2. **Make your changes**:

   - Write clear, documented code
   - Follow the existing code style
   - Add tests for new functionality

3. **Run tests**:

   ```bash
   go test ./...
   ```

4. **Commit your changes**:

   ```bash
   git commit -m "feat: add awesome new feature"
   ```

   We follow [Conventional Commits](https://www.conventionalcommits.org/):

   - `feat:` New feature
   - `fix:` Bug fix
   - `docs:` Documentation only
   - `refactor:` Code change that neither fixes a bug nor adds a feature
   - `test:` Adding or correcting tests
   - `chore:` Maintenance tasks

5. **Push and create a Pull Request**:
   ```bash
   git push origin feature/your-feature-name
   ```
   Then open a PR on GitHub.

## 🎨 Code Style Guidelines

### Go Code

- Follow standard Go formatting (`go fmt`)
- Use meaningful variable and function names
- Add comments for exported functions
- Keep functions focused and small

### JavaScript

- Use `const` and `let`, avoid `var`
- Use async/await for asynchronous operations
- Keep functions small and focused
- Use descriptive variable names

### HTML/CSS

- Use semantic HTML elements
- Follow the existing dark theme styling
- Use CSS variables for colors (`var(--accent-color)`)
- Keep accessibility in mind

## 🧪 Testing

### Running Tests

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific test
go test -run TestFunctionName ./internal/logic
```

### Writing Tests

- Place tests in `*_test.go` files
- Use table-driven tests where appropriate
- Test edge cases and error conditions

## 📦 Building Releases

To build for all platforms:

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

## 💬 Getting Help

- Open an issue for bugs or feature requests
- Check existing issues and discussions
- Be respectful and constructive

## 📄 License

By contributing, you agree that your contributions will be licensed under the [MIT License](LICENSE).

---

Thank you for contributing to GitHousekeeper! 🏠
