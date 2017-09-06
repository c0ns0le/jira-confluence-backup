# Atlassian backup utility

This utility allows taking backups of atlassian jira and confluence instances.
We tested this against atlassian cloud, so no idea if it works against self hosted version.

WARNING: Please be aware that the login might use a non-email username. 
Confluence doesn't allow backups to be taken with the email as a username. 
Jira seems to be fine with the email.

**IMPORTANT:** Use at your own risk. 
We take no responsability for the correctness of this utility. 
Always remember to test that you can actually restore your backups..
This is a very fiddly solution since there is no officially supported API to perform backups.


# Compiling

Make sure you have a golang and make installed

Building all binaries: (will output then in `bin/`)

```
make
```

**NOTE:** Only tested this on mac osx and linux...

# Creating the docker build

This also pushes the build image to quay.io (provided you are authorized :D)

```
make docker
```

# Usage

```
➜  jira-confluence-backup

Please specify if you want to backup jira or confluence

Usage of ./jira-confluence-backup:
      --jira               Perform a backup of JIRA
      --confluence         Perform a backup of Confluence
      --attachments        Backup attachments (default true)
      --exporttocloud      Perform a backup that can be restored in the cloud (default true)
      --file string        File to store the backup in (default "./backup.zip")
      --timeout duration   Timeout wait for the backup, eg: 2h45m (default 3h0m0s)
      --url string         Url of the jira/confuence instance
      --user string        User to authenticate against atlassian
      --pass string        Password to authenticate with

```

# Environment variables

You can also pass the user and password using environment vars in you scripts:

```
export ATL_USER="myusername"
export ATL_PASS="mypassword"
```

If a backup fails, the exit code should be non-zero, so you can check in scripts for that.


# Example of backing up JIRA

```
➜  export ATL_USER="myusername"
➜  export ATL_PASS="mypassword"
➜  jira-confluence-backup --jira --url https://[SOMEACCOUNT].atlassian.net
```

# Example of backing up CONFLUENCE

```
➜  export ATL_USER="myusername"
➜  export ATL_PASS="mypassword"
➜  jira-confluence-backup --confluence --url https://[SOMEACCOUNT].atlassian.net
```
