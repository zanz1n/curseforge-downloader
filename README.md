# Curseforge Downloader

A simple resource made to download curseforge modpack files by cli.

## Usage

```bash
# The api key can also be set by the env CURSEFORGE_API_KEY
curseforge-dl --api-key "PUT THE KEY HERE"
```

### Params

- "--api-key" Required if the environment variable CURSEFORGE_API_KEY is not set
- "--file-path" The path to the curseforge manifest.json
- "--out" The output directory for the downloads
