# Project Bootstrapping Guide

`shroudenv` allows you to declare and initialize environment variables and secrets for a project using a declarative scaffolding configuration file: `.shroudenv.yaml`. 

This guide explains the YAML format, schema features, and how to bootstrap your project.

---

## 📋 The Scaffolding File (`.shroudenv.yaml`)

The `.shroudenv.yaml` file resides in your project's root directory and lists the schema, types, defaults, validations, and generation rules for all required environment variables.

### Configuration Properties

| Property | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `version` | `string` | **Yes** | The configuration format version. Currently must be `"1"`. |
| `project` | `string` | **Yes** | The unique identifier/name of the project (e.g. `my-web-app`). |
| `default_environment` | `string` | No | The default target environment (e.g. `development`) if none is specified. |
| `variables` | `array` | **Yes** | A list of environment variable definitions. |

---

## 🗂️ Variable Schema definitions

Each item in the `variables` block has the following properties:

| Property | Type | Description |
| :--- | :--- | :--- |
| `name` | `string` | **Required.** The environment variable key name (e.g., `DATABASE_URL`). Must start with a letter/underscore and contain only alphanumeric characters/underscores. |
| `description` | `string` | Explains the variable's purpose to other developers. |
| `type` | `string` | The variable type for validation. One of: `string` (default), `integer`, `number`, or `boolean`. |
| `default` | `any` | The default fallback value if no value is entered or inherited. |
| `prompt` | `string` | A custom description or question presented during interactive prompts. |
| `sensitive` | `boolean` | If `true`, the CLI masks your keystrokes (no echoing) during interactive entry. |
| `optional` | `boolean` | If `true`, permits bootstrapping to succeed even if the value is left empty. |
| `fallback` | `string` | A host environment variable to read before prompting. (e.g. `fallback: "PORT"`). |
| `validation` | `object` | An optional set of criteria that the input value must satisfy (see below). |
| `generator` | `object` | Automatically generates a secure value instead of prompting the user (see below). |

---

## 🔍 Validation Constraints

You can define rules to ensure that variables match expected formats (e.g., integer ranges, string patterns, enums) under the `validation` property:

```yaml
validation:
  min: 1024                    # Minimum value (applicable to integers and numbers)
  max: 65535                   # Maximum value (applicable to integers and numbers)
  enum:                        # Exact list of permitted string values
    - "http"
    - "https"
  pattern: "^https?://"        # Regular expression pattern the value must match
  error_message: "Must be URL" # Custom message printed when validation fails
```

> [!NOTE]
> Custom `error_message` overrides only apply when a regular expression `pattern` check fails. Native type conversions (e.g. entering `abc` for an integer) will print clear type mismatch errors automatically.

---

## 🎲 Automated Secret Generators

You can automate secret generation for items like passwords, salts, API keys, or JWT secrets by using the `generator` block. If a variable contains a `generator` rule, `shroudenv` resolves it automatically and does not prompt the user.

```yaml
generator:
  type: "random_string"       # Generator engine. One of: "uuid", "random_string"
  length: 32                  # Length of the string (applicable to random_string)
  charset: "alphanumeric"     # Characters to use (alphanumeric, alphabet, digits, hex)
  encoding: "hex"             # Target encoding (hex, base64, base32, url)
```

---

## 🚀 Step-by-Step Guide

### Step 1: Create `.shroudenv.yaml`
Add a `.shroudenv.yaml` file to the root of your application repository. Here is a realistic template:

```yaml
version: "1"
project: "my-app"
default_environment: "development"
variables:
  - name: PORT
    type: integer
    default: 3000
    validation:
      min: 1024
      max: 49151

  - name: DATABASE_URL
    type: string
    prompt: "Enter SQLite Database connection string"
    default: "Data Source=/app/data/db.sqlite"

  - name: SESSION_SECRET
    description: "Encryption secret for web sessions"
    generator:
      type: random_string
      length: 32
      encoding: "hex"

  - name: OPENAI_API_KEY
    type: string
    sensitive: true
    fallback: "ENV_OPENAI_API_KEY"
```

### Step 2: Initialize shroudenv (First Time)
Ensure that you have initialized your vault on your system:
```bash
shroudenv init
```

### Step 3: Run Bootstrap
Execute the bootstrap command in the directory containing the `.shroudenv.yaml` file:

```bash
shroudenv bootstrap
```

`shroudenv` will perform the following actions:
1. Load your local `~/.shroudenv/db.json` database.
2. Read the `.shroudenv.yaml` configuration.
3. Automatically generate secrets (e.g., `SESSION_SECRET`).
4. Read environment variable fallbacks from the host OS (e.g., if `ENV_OPENAI_API_KEY` is present in your shell, it will use its value).
5. Interactively prompt you for any remaining unassigned variables.
6. Encrypt the gathered secrets and store them under the project `"my-app"` and environment `"development"`.

> [!TIP]
> You can choose a different target environment name (e.g. `staging`) during bootstrap using the `-e` flag:
> ```bash
> shroudenv bootstrap -e staging
> ```

### Step 4: Launch your Application
Run your application using the `shroudenv inject` command. Your application will start with the decrypted variables injected directly into its RAM:

```bash
shroudenv inject -p my-app -e development -- npm run start
```


