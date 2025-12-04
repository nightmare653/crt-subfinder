# 🔎 crt-subfinder (Go)

A fast, recursive subdomain enumerator that uses **crt.sh Certificate Transparency logs** to discover subdomains and wildcard roots.
Supports **concurrency**, **configurable rate limiting**, **retries**, **timeout control**, and **skip-done mode**.

---

## 📌 Features

* ✅ Recursively discovers subdomains from **crt.sh**
* ✅ Extracts and follows **wildcard roots** automatically
* ✅ **Concurrency support** (`-workers`) for faster enumeration
* ✅ Adjustable:

  * Rate limit (`-rate`)
  * Retries (`-retries`)
  * Timeout (`-timeout`)
* ✅ Skip previously scanned domains (`-skip-done`)
* ✅ Output saved cleanly per domain:

  * `subs.txt`
  * `wildcards_clean.txt`

---

## 📦 Installation

### Clone and build:

```bash
git clone https://github.com/yourrepo/crt-subfinder
cd crt-subfinder
go build -o crt_subfinder main.go
```

---

## 📁 Input Format

Create a file like `targets.txt`:

```
example.com
wien.gv.at
aliexpress.com
```

Lines starting with `#` are ignored.

---

## 🚀 Usage

### **Basic Run**

```bash
./crt_subfinder targets.txt
```

---

## ⚙️ Flags

| Flag         | Description                                     | Default |
| ------------ | ----------------------------------------------- | ------- |
| `-workers`   | Number of concurrent workers (1 = sequential)   | `1`     |
| `-rate`      | Delay (seconds) between crt.sh requests         | `1`     |
| `-retries`   | Max retry attempts per request                  | `3`     |
| `-skip-done` | Skip domains that already have non-empty output | `true`  |
| `-timeout`   | HTTP timeout in seconds                         | `20`    |

---

## ⭐ Recommended command (fast + stable)

```bash
./crt_subfinder -workers 5 -rate 2 -retries 5 -skip-done=true -timeout 60 targets.txt
```

This configuration:

* Uses **5 workers** for speed
* Waits **2 seconds** between requests (polite to crt.sh)
* Retries **5 times**
* Allows **60 seconds** to avoid timeout errors
* Skips already processed domains

---

## 📂 Output Structure

After running, each domain gets its own folder:

```
example.com/
├── subs.txt
└── wildcards_clean.txt
```

### `subs.txt`

Contains unique discovered subdomains.

Example:

```
www.example.com
api.example.com
mail.example.com
```

### `wildcards_clean.txt`

Contains wildcard roots discovered from crt.sh.

Example:

```
dev.example.com
shop.example.com
```

These roots are recursively scanned.

---

## 🔄 How Recursive Enumeration Works

If crt.sh returns:

```
*.shop.example.com
```

The tool:

1. Extracts `shop.example.com`
2. Saves it in `wildcards_clean.txt`
3. Adds it to the processing queue
4. Recursively queries for:

   * `*.shop.example.com`
   * All resulting subdomains
   * Any new wildcard roots

This continues until no new roots are found.

---

## ❗ Notes

* Respect crt.sh — avoid using very high concurrency (e.g., 50 workers).
* For large targets, increase `-timeout` and `-rate`.

---

## 📜 Example Full Workflow

```bash
echo "example.com" > targets.txt
echo "wien.gv.at" >> targets.txt

./crt_subfinder -workers 5 -rate 2 -retries 5 -skip-done=true -timeout 60 targets.txt
```

Results:

```
example.com/subs.txt
example.com/wildcards_clean.txt
wien.gv.at/subs.txt
wien.gv.at/wildcards_clean.txt
```

---

## 🛠️ Future Improvements (optional)

* Add `cobra` CLI structure (`crt-subfinder scan`, `crt-subfinder version`)
* Add DNS resolution / IP enrichment
* Add output to JSON/CSV
* Add multiple CT sources (certspotter, google, etc.)

---
