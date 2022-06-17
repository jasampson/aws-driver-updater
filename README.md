# aws-updater-go

This project contains source and binary written in Go to update AWS PV, ENA and NVME Windows drivers on AWS EC2 instances automatically.

## Getting started

Simply [download the executable from here](./release/aws-updater-go.exe) and run it on an EC2 instance.

## What does this program do exactly?

By default this program will simply check AWS EC2 ENA/NVME/PV driver versions and produce a report.

To install drivers you must pass the `-i` arg to the executable.

Updating drivers will likely cause momentary network disconnections while the drivers update. It is recommended by AWS to reboot your EC2 instance after updating any drivers.

### Without passing any args

1. Checks the AWS website for the latest available driver versions.
2. Checks the installed AWS driver versions. The check includes determining if the EC2 instance type is compatible with NVME and ENA drivers.
3. Determines if installed AWS drivers are up to date and prints a table with all driver versions and if any updates are available.

### If you pass in the `-i` arg

1. Checks the AWS website for the latest available driver versions.
2. Checks the installed AWS driver versions. The check includes determining if the EC2 instance type is compatible with NVME and ENA drivers.
3. Determines if installed AWS drivers are up to date and prints a table with all driver versions and if any updates are available.
4. Downloads updates in parallel from the AWS website.
5. Extracts update files from zip in parallel to the current directory.
6. Runs the AWS driver installer silently as needed.
7. Cleans up any download files/directories.

## Compiling a new version

Normally Gitlab CI/CD will handle building a release automatically when a new revision is checked in to the git repo. If you do need to manually build an executable you can do it this way

1. [Download and install](https://go.dev/doc/install) Go for your platform of choice. You can use any platform as long as your version is at least 1.18.2
2. Clone this git repo to your local storage. You can click the blue Clone button above for instructions.
3. Open up a shell/terminal and navigate to the directory you cloned this project into.
4. Compile the binary. This will create aws-updater-go.exe.

   a. Windows can compile using this command `go build`

   b. Linux and MacOS can compile using this command: `GOOS=windows GOARCH=amd64 go build`
