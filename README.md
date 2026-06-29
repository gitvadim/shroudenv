# shroudenv

> **Secure, offline, local-first environment variable and secrets manager for developers.**

[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

![Dashboard Screenshot](./docs/images/shroudenv-screenshot.png)

`shroudenv` allows you to isolate credentials by project and environment (e.g., development, staging, production) and inject them securely into sub-processes without ever writing them to a `.env` file on disk.

---

## 🔒 Why shroudenv?

If you are a developer, you probably deal with `.env` files every day. But `.env` files have major drawbacks:
* **Security Risks:** Secrets are stored in plaintext on your hard drive, making them vulnerable to accidental Git commits, backup leaks, or local process snooping.
* **Synchronization Pain:** Keeping variables in sync across multiple projects, environments (development, staging, production), and setups is tedious and error-prone.
* **Portability Issues:** Managing multiline variables (e.g., private keys) or escape characters in standard `.env` files is clunky and platform-dependent.

`shroudenv` solves this by giving you:
1. **100% Offline & Private:** Zero external network calls. No cloud dependencies, subscription accounts, or third-party tracking. Your data stays entirely yours.
2. **Zero Dependencies & Friction:** No external database engines, setups, or complex installation steps required. Just download the single lightweight binary and run.
3. **OS Keyring Security:** Your master key is generated randomly and stored securely in your OS vault (Windows Credential Manager, macOS Keychain, or Linux Secret Service).
4. **AES-256-GCM Encryption:** Your offline secrets database is fully encrypted on disk.
5. **Subprocess Injection:** Inject decrypted environment variables directly into child processes at runtime. Your secrets stay in RAM and **never** touch your hard drive in plaintext.

---

## 🚀 Quick Start

### 1. Install
Download the latest binary from the [Releases](https://github.com/gitvadim/shroudenv/releases) page or build from source.

### 2. Initialize
Create your secure vault and generate your master key:
```bash
./shroudenv init
```

### 3. Launch Dashboard
Start the local server and open the modern GUI dashboard:
```bash
./shroudenv ui
```

---

## ✨ Key Features

* **Local-First & Offline:** Secrets are stored fully offline on your own machine.
* **Dual Interface:** Full-featured CLI for terminal/script integrations, plus an embedded Web GUI dashboard.
* **Zero-Disk Injection:** Run application servers or scripts with credentials loaded directly into memory. No more `.env` files leaked in git or left on disk.
* **Embedded Frontend:** The React dashboard is compiled directly into the Go executable for a single-binary experience.
* **`.env` File Ingestion:** Import `.env` files via the dashboard with live previews and merge strategies.

---

## Security Model & Hardening

* **Vault-Backed Master Key:** Generates a secure random 256-bit master key stored directly in your OS credential store (Windows Credential Manager, macOS Keychain, Linux Secret Service).
* **AES-256-GCM Cryptography:** Secrets are encrypted on disk in `~/.shroudenv/db.json` using AES-256-GCM. Metadata (project names, environments) remains plain-text for indexing.
* **Headless Fallbacks:** In non-interactive contexts (like Docker or CI), falls back to `SHROUDENV_MASTER_KEY` env var or prompts for a master password derived via `scrypt`.
* **Host Header Validation (DNS Rebinding Protection):** The API server binds strictly to `localhost` / `127.0.0.1` and blocks any request with an external Host header (returns `403 Forbidden`).
* **Session Token Authentication:** Local dashboard communication is protected by a secure 32-character token generated on server startup.
* **Automated Secret Harvesting Protections (TTY Enforcement):** `secret list` and `inject` commands fail-securely if standard output/input is redirected or piped. A `--non-interactive` flag or `SHROUDENV_NON_INTERACTIVE=true` must be supplied for automation.

---

## Technical Stack

* **Backend & CLI:** Go (Golang), `github.com/spf13/cobra` (CLI), `github.com/zalando/go-keyring` (cross-platform keychain integration).
* **Frontend:** React, Vite, TypeScript, Vanilla CSS (glassmorphic dark-theme, responsive layouts, Lucide icons).

---

## Setup & Build

### Prerequisites
* Go 1.20+
* Node.js 18+ & npm

### Build Instructions

1. **Build the Frontend Assets:**
   ```bash
   # Navigate into the frontend folder
   cd frontend
   # Install dependencies
   npm install
   # Compile and build the React app
   npm run build
   cd ..
   ```

2. **Compile the Go Executable:**
   This embeds the compiled React static files under `frontend/dist/` into the Go binary.
   ```bash
   go build -o shroudenv.exe main.go
   ```

---

## CLI Reference

### 1. Storage Initialization
Generates a new master key, saves it in the keyring, and sets up the local database structure.
```bash
./shroudenv init
```

### 2. Launching the Web GUI
Launches the local net/http server, prints the URL with the secure access token, and opens the dashboard in your default browser.
```bash
./shroudenv ui
```

### 3. Project Management
Create and list secure project spaces.
```bash
# Create a project
./shroudenv project create my-api

# List projects
./shroudenv project list
```

### 4. Environment Management
Create and list environment profiles inside a project.
```bash
# Create environment
./shroudenv env create my-api production

# List environments
./shroudenv env list my-api
```

### 5. Secrets Management
Configure and list credentials.
```bash
# Set a secret key/value
./shroudenv secret set my-api production DATABASE_URL "postgres://user:pass@localhost/db"

# List secrets (displays in KEY=VALUE format; requires TTY or override)
./shroudenv secret list my-api production --non-interactive
```

### 6. Subprocess Injection
Spawns your command, injecting decrypted environment variables directly into the child process. This means your secrets stay in memory and never touch a `.env` file on disk.

```bash
./shroudenv inject -p my-api -e production --non-interactive -- npm run dev
```

## 🗺️ Roadmap & Future Features

We are planning to expand `shroudenv` to support team collaboration and enterprise environments.

> [!IMPORTANT]
> **Our Philosophy: Zero Shovelware, Zero Forced Cloud Accounts**
> Unlike other tools that transitioned from local-first utilities to forced cloud models, `shroudenv` will **always** remain a lightweight, fully functional offline-first utility. Team and enterprise features (like SSO, RBAC, and cloud sync) will be designed as strictly **opt-in and modular extensions**. The core developer experience will remain completely offline, private, and bloat-free.

### Upcoming Features
* **Offline Secrets Sharing:** Share environment variables securely and fully offline. Export any environment as an encrypted, compressed base64 payload. Share the payload along with its decryption key via your communication tools of choice (Teams, Slack, Email) for direct import.

### Enterprise Extensions (Planned)
* **Secure Team Sharing:** End-to-end encrypted sharing of project secrets between team members, without storing decryption keys on centralized servers.
* **Centralized Configuration:** Sync local vaults with central, self-hosted storage backends featuring strict audit logging.
* **Role-Based Access Control (RBAC):** Define explicit read/write access policies for staging vs. production environments.
* **Single Sign-On (SSO):** Authenticate dashboard access with enterprise identity providers (SAML, OIDC).


---

## 🛠️ Contributing

We welcome contributions! Whether it's a bug report, a feature request, or a pull request, feel free to help make `shroudenv` better.

1. **Fork the repository**
2. **Create a feature branch** (`git checkout -b feature/AmazingFeature`)
3. **Commit your changes**
4. **Push to the branch**
5. **Open a Pull Request**

## 📄 License

This project is licensed under the MIT License - see the `LICENSE` file for details.
