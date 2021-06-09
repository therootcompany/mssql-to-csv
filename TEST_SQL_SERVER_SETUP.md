# Mirror of The Root Blog

See <https://therootcompany.com/blog/mssql-server-on-ubuntu/>.

Preserved here in the repo for all of our sanity.

# How to create a SQL Server test instance

1. Set up a fresh 2GB VPS on [Digital Ocean](https://m.do.co/c/18ec10e74dae)
2. `free -m` to make sure you have at least **2GB of RAM**
3. Use **Ubuntu 20.04 LTS** for this documentation

## Microsoft's documentation

This is just my abbreviation of the official Microsoft Documentation for running SQL Server on Ubuntu:
<https://docs.microsoft.com/en-us/sql/linux/quickstart-install-connect-ubuntu?view=sql-server-ver15>

You may want to check that if this becomes out of date.

## Get Microsoft's Repo Keys

SQL Server is not in the official Ubuntu / Debian software app store (repository).

You will need to add Microsoft's app store to your Ubuntu Linux instance, like so:

```bash
wget -qO- https://packages.microsoft.com/keys/microsoft.asc | sudo apt-key add -
sudo add-apt-repository "$(wget -qO- https://packages.microsoft.com/config/ubuntu/20.04/mssql-server-2019.list)"
sudo apt-get update
```

## Install Microsoft SQL Server

Now that it's in the app store (repository), you can istall it by name:

```bash
sudo apt-get install -y mssql-server
```

You'll need a new password. May I suggest a nice, long, 192-bit secure random password?

```bash
xxd -l24 -ps /dev/urandom | xxd -r -ps | base64 \
    | tr -d = | tr + - | tr / _
```

**IMPORTANT**: You're best to avoid special characters, otherwise you'll need to URL-escape them later.

Then select a license and set the password to finish setting up the server:

```bash
sudo /opt/mssql/bin/mssql-conf setup
# Developer Edition
systemctl status mssql-server --no-pager
```

If the server failed to start, it's probably because you don't have 2GB of RAM.

## SQL Server CLI Tools

Again, add the Microsoft repository and signing keys:

```bash
curl https://packages.microsoft.com/keys/microsoft.asc | sudo apt-key add -
curl https://packages.microsoft.com/config/ubuntu/20.04/prod.list | sudo tee /etc/apt/sources.list.d/msprod.list
sudo apt-get update
```

Then you can install the tools:

```bash
sudo apt-get install mssql-tools unixodbc-dev
```

```bash
echo 'export PATH="$PATH:/opt/mssql-tools/bin"' >> ~/.bashrc
source ~/.bashrc
```

## Testing that it all worked

Connect to the server (use the password you created above):

```bash
sqlcmd -S localhost -U SA
```

Do a basic check that you can create some data:

```
CREATE DATABASE TestDB;
SELECT Name from sys.Databases;
USE TestDB;
GO;

CREATE TABLE TestTable1 (id CHAR(36), name VARCHAR(255), attr VARCHAR(255));
INSERT INTO TestTable1 VALUES ('xyz', 'banana', 'tasty');
INSERT INTO TestTable1 VALUES ('abc', 'orange', 'sweet');
GO;

QUIT;
```

If you want to read from a file you do so like this:

```bash
sqlcmd -S localhost -U SA -i fixtures.sql
```

## Credentials & Connection String

Here's what a SQL Server connection string looks like:

```txt
sqlserver://MY_USER@MY_PASS:MY_HOST/MY_INSTANCE?database=MY_CATALOG&Encrypt=disable
```

**Note**: SQL Server 2008 has some janky TLS that doesn't play well with some database drivers.

It's broken down into these components:

```bash
# This is the IP Address
MSSQL_SERVER=localhost

# This is the default port
MSSQL_PORT=1433

# SA is the default admin. Don't use it. :p
MSSQL_USERNAME=SA

# This may need to be URL-escaped if it has special characters
MSSQL_PASSWORD=Password1

# I think you leave this empty for TCP/IP connections
MSSQL_INSTANCE=""

# This is the database name
MSSQL_CATALOG="TestDB"

# If you need them
MSSQL_PARAMS="Encrypt=true"
```
