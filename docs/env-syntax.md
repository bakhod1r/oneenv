---
title: .env file syntax
layout: default
nav_order: 7
---

# `.env` file syntax

`oneenv` supports the syntax you expect from a mature `.env` parser:

```dotenv
# A comment line.
export PATH_STYLE=ok            # "export " prefix is allowed; inline comments too

PLAIN=value
QUOTED="double quoted"          # escapes: \n \r \t \" \\
RAW='single quoted'             # no escapes, no expansion — taken literally
MULTILINE="line one
line two"                       # newlines allowed inside double quotes

GREETING="Hello ${USER}"        # ${VAR} / $VAR expansion — only with WithExpand()
LITERAL='$NOT_EXPANDED'         # single quotes never expand
```

- **Comments** — a `#` starting a line, or preceded by whitespace on a value line.
- **`export ` prefix** — accepted and ignored, so you can `source` the same file.
- **Quotes** — double quotes honour escapes and can span multiple lines; single quotes are literal.
- **Expansion** — `${VAR}` and `$VAR` are expanded (when `WithExpand()` is set) against values already parsed in the file, falling back to the process environment. Write `$$` for a literal `$`.

Syntax errors come back as a [`*ParseError`](errors) carrying the file name and
line number.

## Environment-aware file cascade

`WithEnvFiles()` layers files by the active environment, the convention used by
Rails, Next.js and dotenv-cli. On top of each base file it also reads
`<base>.local`, `<base>.<env>` and `<base>.<env>.local`, each optional, later
files overriding earlier keys:

```go
cfg, err := oneenv.Parse[Config](oneenv.WithEnvFiles())
// with APP_ENV=production, reads in increasing priority:
//   .env  →  .env.local  →  .env.production  →  .env.production.local
```

The environment name comes from `APP_ENV`, then `GO_ENV`; change the sources
with `WithEnvVar("MY_ENV")`. `FilesFor(opts...)` returns the resolved list.
