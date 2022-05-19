# aws-updater-go
This project contains source written in Go to automatically update AWS PV, ENA and NVME Windows drivers on AWS EC2 instances automatically.
## Getting started
You will need to compile the binary yourself using Go.
1. [Download and install](https://go.dev/doc/install) Go for your platform of choice.  You can use any platform as long as your version is at least 1.18.2
2. Clone this git repo to your local storage.  You can click the blue Clone button above for instructions.
3. Open up a shell/terminal and navigate to the directory you cloned this project into.
4. Compile the binary.  This will create aws-updater-go.exe.
    a. Windows users can compile using this command `go build`
    b. Linux and MacOS users can compile using this command: `GOOS=windows GOARCH=amd64 go build`

## Disclaimer
While this has been tested and works for me, there is no warranty whatsoever and it may break everything.  If something is badly broken please feel free to open an issue or submit a PR.
