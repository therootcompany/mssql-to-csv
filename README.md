# mssql-to-csv

A small, lightweight self-contained Go program that eats MS SQL for breakfast
and spits out CSV for lunch.

# Features

- [x] From SQL to CSV (with proper escapes)
- [x] Map from SQL columns to CSV fields - see [`map.txt`](/map.txt)
- [x] Upload directly to S3 - see [`example.env`](/example.env)
- [x] Run continuously in a loop (ex: `REPORT_FREQUENCY=1h`)

# Usage

```pwsh
mssql-to-csv.exe --help
```

Example:

```pwsh
mssql-to-csv.exe --env env.txt --map map.txt --log log.txt
```

To see the full CSV output in the logs add `--debug`:

```pwsh
mssql-to-csv.exe --env env.txt --map map.txt --log log.txt --debug
```

## Configuration

### Secrets

See [`example.env`](/example.env).

On Windows use WordPad and edit the file as `env.txt`.

**IMPORTANT**

- Password MUST be URL-safe, or URL-escaped (ex: no `@`)
- SQL Server 2008 has TLS/SSL issues. Append `&Encrypt=disable` to the
  `MSSQL_CATALOG` (database name) to skip encryption, otherwise you get this:
  ```txt
  wsarecv: An existing connection was forcibly closed by the remote host
  ```

### DB Column to CSV Field Mappings:

See [`map.txt`](/map.txt).

`map.txt` will define which columns will be exported (if they do not exist, they
will be left blank), and in which order.

```txt
CSV Field Name:  DatabaseColumnName
```

For example:

```txt
Name of Fruit:       FruitName
Type of Fruit:       FruitType
Extra Info:          DOESNT_EXIST
Quantity of Fruit:   FruitQuantity
```

```csv
Name of Fruit,Type of Fruit,Extra Info,Quantity of Fruit
Apple,Sweet or Sour,,7
Orange,Citrus,,3
```

Note that the order is important.

# Install

You will need:

- `mssql-to-csv.exe` (Windows) or `mssql-to-csv` (Mac, Linux)
  - Note: You can grab the latest version for your platform on the
    [Releases Page](https://github.com/therootcompany/mssql-to-csv/releases)
  - You may need to place the binary in a folder in your system's `PATH`
- [`map.txt`](/map.txt)
- `.env`, which can be modeled after [`example.env`](/example.env)
  - Note: Do **NOT** commit secrets, tokens, passwords, etc to git!!

# Build

Here's how you build from source.

## STOP: Do you need to build this?

In most instances you do **NOT** need to build from source. You can get a
pre-built version from the releases page.

## Install Go

If you do need to build this, you'll need `go` and `goreleaser`.

Install `go` (and follow the onscreen instructions):

Mac, Linux:

```bash
curl -sS https://webinstall.dev/go | bash
```

Windows 10:

```pwsh
curl.exe -A MS https://webinstall.dev/go | powershell
```

## Windows System Service

## Task Schedule

1. Search Task Scheduler
2. Create Task...
3. (that brings you to Properties automatically)
4. General Tab
   - Give descriptive name
   - Run whether user is logged on or not
   - (can run as a service account, does not need admin permissions)
5. Triggers Tab
   - Daily
6. Actions Tab
   - New
     - Program/Script: `C:\Path\To\mssql-to-csv.exe`
     - Add Arguments:
       ```txt
       --env env.txt --map map.txt --log /Path/To/Log.txt
       ```
     - Start in `C:\Path\To\`
7. Settings Tab
   - Stop the task if it runs longer than: `1 hour`

## nssm

1. Download [nssm.exe](https://nssm.cc/release/nssm-2.24.zip)
2. Open a Command Prompt
3. Register system service
   ```pwsh
   nssm install mssql-to-csv mssql-to-csv.exe --env .env --map map.txt --log log.txt
   ```
4. Start the service
   ```pwsh
   nssm start mssql-to-csv
   ```
5. Set service to start on boot
   ```pwsh
   nssm set Start Automatic
   ```

## One off build (ex: for Windows)

When you need to build for testing, you can do so like this

```pwsh
git clone git@github.com:therootcompany/mssql-to-csv.git
pushd ./mssql-to-csv/
```

```pwsh
# Build for Windows Command Prompt
go build -v -mod=vendor -race -o mssql-to-csv.debug.exe .

# Build for headless Windows system service
go build -v -mod=vendor -race -ldflags "-H windowsgui" -o mssql-to-csv.exe .
```

## Create a Release

When you want to create a release, tag it, and then run `goreleaser`

Install `goreleaser`:

```bash
curl -sS https://webinstall.dev/goreleaser | bash
```

Test build:

```bash
goreleaser --snapshot --skip-publish --rm-dist
```

Proper release build

```bash
# check existing version tags
git tag

# create a new version, such as v1.0.37, after committing and testing
git tag v1.x.y

# release it to github (working git directory must be clean)
goreleaser --rm-dist
```

Note: You should have a Github Personal Access Token in the file at
`~/.config/goreleaser/github_token`. \
The contents should be just the token, which is a random hexadecimal string that
looks like this: `48b8b13473965ead05889fa073530c630ca70da42`

# LICENSE

This Source Code Form is subject to the terms of the Mozilla Public \
License, v. 2.0. If a copy of the MPL was not distributed with this \
file, You can obtain one at http://mozilla.org/MPL/2.0/.

Copyright 2021 AJ ONeal \
Copyright 2021 The Root Group, LLC
