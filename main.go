package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/go-version"
	"golang.org/x/exp/slices"
)

// used for --version arg, this gets set with linker flags from the build command
var programVersion = "1.0.0"

func unzipSource(source, destination string) error {
	// 1. Open the zip file
	reader, err := zip.OpenReader(source)
	if err != nil {
		return err
	}

	// 2. Get the absolute destination path
	destination, err = filepath.Abs(destination)
	if err != nil {
		return err
	}

	// 3. Iterate over zip files inside the archive and unzip each of them
	for _, f := range reader.File {
		err := unzipFile(f, destination)
		if err != nil {
			return err
		}
	}

	// 4. remove zip file
	reader.Close()
	err = os.Remove(source)
	if err != nil {
		log.Fatal(err)
	}

	return nil
}

func unzipFile(f *zip.File, destination string) error {
	// 4. Check if file paths are not vulnerable to Zip Slip
	filePath := filepath.Join(destination, f.Name)
	if !strings.HasPrefix(filePath, filepath.Clean(destination)+string(os.PathSeparator)) {
		return fmt.Errorf("invalid file path: %s", filePath)
	}

	// 5. Create directory tree
	if f.FileInfo().IsDir() {
		if err := os.MkdirAll(filePath, os.ModePerm); err != nil {
			return err
		}
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(filePath), os.ModePerm); err != nil {
		return err
	}

	// 6. Create a destination file for unzipped content
	destinationFile, err := os.OpenFile(filePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, f.Mode())
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	// 7. Unzip the content of a file and copy it to the destination file
	zippedFile, err := f.Open()
	if err != nil {
		return err
	}
	defer zippedFile.Close()

	if _, err := io.Copy(destinationFile, zippedFile); err != nil {
		return err
	}
	return nil
}

func getLatestVersion(url, regexPattern string) string {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	// Make request
	response, err := client.Get(url)
	if err != nil {
		log.Fatal(err)
	}
	defer response.Body.Close()

	// Copy data from the response to standard output
	// Get the response body as a string
	dataInBytes, err := io.ReadAll(response.Body)
	if err != nil {
		log.Fatal(err)
	}
	pageContent := string(dataInBytes)

	re := regexp.MustCompile(regexPattern)

	parts := re.FindAllString(pageContent, -1)

	matches := []string{}
	for _, part := range parts {
		if !slices.Contains(matches, part) {
			matches = append(matches, part)
		}
	}

	// sort versions so we can find the highest one
	sort.Strings(matches)
	/* uncomment these to debug stuff
	fmt.Println(matches[len(matches)-1])
	fmt.Println(matches)
	*/

	return matches[len(matches)-1]
}

func isAdmin() bool {
	_, err := os.Open("\\\\.\\PHYSICALDRIVE0")
	return err == nil
}

func getEC2InstanceType() string {
	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: 2 * time.Second,
	}

	// Make request to EC2 instance metadata service
	var url string = "http://169.254.169.254/latest/meta-data/instance-type"
	response, err := client.Get(url)
	if err != nil {
		return "error"
	}
	defer response.Body.Close()

	// Get the response body as a string
	dataInBytes, err := io.ReadAll(response.Body)
	if err != nil {
		return "error"
	}

	// yes, this is a simplistic check and it's possible if something responds at url above we could get a false positive
	if len(dataInBytes) > 0 {
		return strings.ToLower(strings.TrimSpace(string(dataInBytes)))
	} else {
		return "error"
	}
}

func main() {
	// help text goes here
	const usage = `Usage of test_program:
	-h, --help    			displays this help message
	-v, --version		 	display version information
	-i,           			install all available driver updates.  by default this program only checks if updates are available.
`

	// parse command line args
	var install, displayversion bool
	flag.BoolVar(&install, "i", false, "Install all available updates.")
	flag.BoolVar(&displayversion, "v", false, "Display version information.")
	flag.Usage = func() { fmt.Print(usage) }
	flag.Parse()

	// display version information and exit
	if displayversion {
		fmt.Println("Version:\t", programVersion)
		os.Exit(0)
	}

	// run preliminary checks
	if runtime.GOOS != "windows" {
		fmt.Println("This program only works with Windows. Exiting.")
		os.Exit(1)
	}
	if !isAdmin() {
		fmt.Println("You must run this program as administrator.  Exiting.")
		os.Exit(1)
	}
	var instanceType string = getEC2InstanceType()
	if instanceType == "error" {
		fmt.Println("This program only works on AWS EC2 instances.  Exiting.")
		os.Exit(1)
	} else {
		fmt.Println(instanceType, "EC2 instance type detected.")
	}

	// define aws driver struct values array.  update values below as needed.
	type aws_driver struct {
		name                     string
		downloadUrl              string
		installedVersionCheckCmd string
		installCmd               string
		verCheckUrl              string
		verCheckRegex            string
		latestVersion            string
		needsUpdate              bool
	}

	var aws_drivers = []aws_driver{
		{
			name:                     "nvme",
			downloadUrl:              "https://s3.amazonaws.com/ec2-windows-drivers-downloads/NVMe/Latest/AWSNVMe.zip",
			installedVersionCheckCmd: "$driver_ver = (Get-WmiObject Win32_PnPSignedDriver | ? {$_.Description -match 'AWS NVMe Elastic Block Storage Adapter'}).DriverVersion; if ($driver_ver) {Write-Host $($driver_ver)} else {Write-Host '1.0.0'}",
			installCmd:               `powershell.exe -NoProfile -File AWSNVMe\install.ps1 -NoReboot`,
			latestVersion:            "",
			verCheckUrl:              "https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/aws-nvme-drivers.html",
			verCheckRegex:            `([\d]\.[\d]\.[\d])`,
			needsUpdate:              false,
		},
		{
			name:                     "pv",
			downloadUrl:              "https://s3.amazonaws.com/ec2-windows-drivers-downloads/AWSPV/Latest/AWSPVDriver.zip",
			installedVersionCheckCmd: "(Get-WmiObject -Class win32_Product | ? {$_.name -match 'AWS PV Drivers'}).Version",
			installCmd:               `powershell.exe -NoProfile -File AWSPVDriver\install.ps1 -Quiet -NoReboot`,
			latestVersion:            "",
			verCheckUrl:              "https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/xen-drivers-overview.html",
			verCheckRegex:            `([\d]\.[\d]\.[\d])`,
			needsUpdate:              false,
		},
		{
			name:                     "ena",
			downloadUrl:              "https://s3.amazonaws.com/ec2-windows-drivers-downloads/ENA/Latest/AwsEnaNetworkDriver.zip",
			installedVersionCheckCmd: "(Get-WmiObject Win32_PnPSignedDriver | ? {$_.FriendlyName -match 'Amazon Elastic Network Adapter'}).DriverVersion",
			installCmd:               `powershell.exe -NoProfile -File AwsEnaNetworkDriver\install.ps1`,
			latestVersion:            "",
			verCheckUrl:              "https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/enhanced-networking-ena.html",
			verCheckRegex:            `([\d]\.[\d]\.[\d])`,
			needsUpdate:              false,
		},
	}

	// run functions in parallel
	var wg sync.WaitGroup

	fmt.Printf("Checking AWS website for latest driver versions.. ")
	wg.Add(len(aws_drivers))
	for i, driver := range aws_drivers {
		go func(i int, driver aws_driver) {
			defer wg.Done()
			aws_drivers[i].latestVersion = getLatestVersion(driver.verCheckUrl, driver.verCheckRegex)
		}(i, driver)
	}
	wg.Wait()
	fmt.Println("Done.")

	fmt.Println("Checking installed AWS driver versions.. Done.")
	wg.Add(len(aws_drivers))
	fmt.Println("Type   Installed   Latest      Update Available")
	fmt.Println("---- | --------- | --------- | ----------------")
	for i, driver := range aws_drivers {
		go func(i int, driver aws_driver) {
			defer wg.Done()

			// certain driver types have specific instance type requirements, which we account for here
			var driverVersionCheckEarlyExit bool = false
			switch driver.name {
			case "ena":
				/* ena is only supported on specific instance types outlined here: https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/enhanced-networking-ena.html
				   make sure these are all entered in lowercase */
				var enaInvalidInstanceTypesPrefix = []string{"c4", "d2", "t2"}
				var enaInvalidInstanceTypes = []string{"m4.large", "m4.xlarge", "m4.2xlarge", "m4.4xlarge", "m4.10xlarge"}
				instancePrefix := strings.Split(instanceType, ".")[0]
				if slices.Contains(enaInvalidInstanceTypes, instanceType) || slices.Contains(enaInvalidInstanceTypesPrefix, instancePrefix) {
					driverVersionCheckEarlyExit = true
				}
			case "nvme":
				/* nvme is only supported on nitro systems outlined here https://docs.aws.amazon.com/AWSEC2/latest/WindowsGuide/instance-types.html#ec2-nitro-instances
				   make sure these are all entered in lowercase */
				var nvmeValidInstanceTypesPrefix = []string{"c5", "c5a", "c5ad", "c5d", "c5n", "c6a", "c6i", "c6id", "d3", "d3en", "g4", "g4ad", "g5", "i3en", "i4i", "m5", "m5a", "m5ad", "m5d", "m5dn", "m5n", "m5zn", "m6a", "m6i", "m6id", "r5", "r5a", "r5ad", "r5b", "r5d", "r5dn", "r5n", "r6i", "r6id", "t3", "t3a", "x2idn", "x2iedn", "x2iezn", "z1d"}
				var nvmeValidInstanceTypes = []string{"p3dn.24xlarge", "u-12tb1.112xlarge", "u-3tb1.56xlarge", "u-6tb1.112xlarge", "u-6tb1.56xlarge", "u-9tb1.112xlarge"}
				instancePrefix := strings.Split(instanceType, ".")[0]
				if slices.Contains(nvmeValidInstanceTypes, instanceType) || slices.Contains(nvmeValidInstanceTypesPrefix, instancePrefix) {
					driverVersionCheckEarlyExit = false
				} else {
					driverVersionCheckEarlyExit = true
				}
			}
			// exit early if driver version check fails based on the driver and EC2 instance type
			if driverVersionCheckEarlyExit {
				fmt.Printf("%-4s | %-9s | %-9s | not supported on %s instance type\n", driver.name, "none", "none", instanceType)
				return
			}

			// retrieve driver version using powershell commands defined above
			cmdArgs := []string{"-NoProfile", "-Command", driver.installedVersionCheckCmd}
			cmdOut, err := exec.Command("powershell.exe", cmdArgs...).CombinedOutput()
			if err != nil {
				fmt.Println(err)
			}
			cmdOutClean := strings.TrimSpace(string(cmdOut))
			if len(cmdOutClean) == 0 {
				log.Fatal(driver.name, " driver version not returned")
			}
			/*
				clean up anything beyond 3 subversions mainly for display purposes
				the AWS website does versions like this: 1.4.0
				windows driver versions sometimes do this: 1.4.0.0
				we'll still pass on the extended version to the version comparison, which will do the right thing regardless
			*/
			cmdOutCleanSplit := strings.Split(cmdOutClean, ".")
			cmdOutCleaner := cmdOutCleanSplit[0] + "." + cmdOutCleanSplit[1] + "." + cmdOutCleanSplit[2]

			// perform version comparison and set needsUpdate appropriately
			v1, err := version.NewVersion(driver.latestVersion)
			if err != nil {
				log.Fatal(err)
			}
			v2, err := version.NewVersion(cmdOutClean)
			if err != nil {
				log.Fatal(err)
			}
			if v2.LessThan(v1) {
				aws_drivers[i].needsUpdate = true
			}
			var needsUpdate string
			if aws_drivers[i].needsUpdate {
				needsUpdate = "yes"
			} else {
				needsUpdate = "no"
			}

			fmt.Printf("%-4s | %-9s | %-9s | %s\n", driver.name, cmdOutCleaner, driver.latestVersion, needsUpdate)
		}(i, driver)
	}
	wg.Wait()

	// only continue if -install arg was passed and updatesNeeded = true
	var updatesNeeded bool = false
	for _, driver := range aws_drivers {
		if driver.needsUpdate {
			updatesNeeded = true
		}
	}
	if !updatesNeeded {
		fmt.Println("AWS driver versions are up to date.  Exiting.")
		os.Exit(0)
	} else if updatesNeeded && install {
		fmt.Println("AWS Driver updates are needed.  Beginning installation.")
	} else {
		fmt.Println("AWS Driver updates are needed but -i was not passed.  Exiting.")
		os.Exit(0)
	}

	wg.Add(len(aws_drivers))
	for _, driver := range aws_drivers {
		go func(name, url string, updateNeeded bool) {
			defer wg.Done()
			if updateNeeded {
				tokens := strings.Split(url, "/")
				fileName := tokens[len(tokens)-1]
				fmt.Println("Downloading latest", name, "driver to", fileName)

				output, err := os.Create(fileName)
				if err != nil {
					log.Fatal("Error while creating", fileName, "-", err)
				}
				defer output.Close()

				res, err := http.Get(url)
				if err != nil {
					log.Fatal("http get error: ", err)
				} else {
					defer res.Body.Close()
					_, err = io.Copy(output, res.Body)
					if err != nil {
						log.Fatal("Error while downloading", url, "-", err)
					}
				}
			}
		}(driver.name, driver.downloadUrl, driver.needsUpdate)
	}
	wg.Wait()

	wg.Add(len(aws_drivers))
	for _, driver := range aws_drivers {
		go func(url string, updateNeeded bool) {
			defer wg.Done()
			if updateNeeded {
				urlFileName := url[strings.LastIndex(url, "/")+1:]
				if strings.ToLower(url[len(url)-3:]) == "zip" {
					fmt.Println("Extracting", urlFileName)
					err := unzipSource(urlFileName, urlFileName[:len(urlFileName)-4])
					if err != nil {
						log.Fatal(err)
					}
				}
			}
		}(driver.downloadUrl, driver.needsUpdate)
	}
	wg.Wait()

	for _, driver := range aws_drivers {
		if driver.needsUpdate {
			fmt.Printf("Installing %s version %s driver.. ", driver.name, driver.latestVersion)
			var cFields []string = strings.Fields(driver.installCmd)
			var cmdExe string = cFields[0]
			var cmdArgs []string = cFields[1:]
			_, err := exec.Command(cmdExe, cmdArgs...).CombinedOutput()
			if err != nil {
				log.Fatal(err)
			}
			// change _ above to cmdOut and uncomment this for troubleshooting installer errors: fmt.Println(string(cmdOut))
			fmt.Println("Done.")
		}
	}

	fmt.Printf("Cleaning up.. ")
	for _, driver := range aws_drivers {
		if driver.needsUpdate {
			dirName := driver.downloadUrl[strings.LastIndex(driver.downloadUrl, "/")+1 : len(driver.downloadUrl)-4]
			err := os.RemoveAll(dirName)
			if err != nil {
				log.Fatal(err)
			}
		}
	}
	fmt.Println("Done.")

	fmt.Println("Please reboot to complete driver installation.")

}
