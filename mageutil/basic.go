package mageutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/magefile/mage/sh"
)

// CheckAndReportBinariesStatus checks the running status of all binary files and reports it.
func CheckAndReportBinariesStatus() {
	InitForSSC()
	err := CheckBinariesRunning()
	if err != nil {
		PrintRed("Some programs are not running properly:")
		PrintRedNoTimeStamp(err.Error())
		os.Exit(1)
	}
	PrintGreen("All services are running normally.")
	PrintBlue("Display details of the ports listened to by the service:")
	time.Sleep(1 * time.Second)
	err = PrintListenedPortsByBinaries()
	if err != nil {
		PrintRed("PrintListenedPortsByBinaries error")
		PrintRedNoTimeStamp(err.Error())
		os.Exit(1)
	}
}

// StopAndCheckBinaries stops all binary processes and checks if they have all stopped.
func StopAndCheckBinaries() {
	InitForSSC()
	KillExistBinaries()
	err := attemptCheckBinaries()
	if err != nil {
		PrintRed(err.Error())
		return
	}
	PrintGreen("All services have been stopped")
}

func attemptCheckBinaries() error {
	const maxAttempts = 15
	var err error
	for i := 0; i < maxAttempts; i++ {
		err = CheckBinariesStop()
		if err == nil {
			return nil
		}
		PrintYellow("Some services have not been stopped, details are as follows: " + err.Error())
		PrintYellow("Continue to wait for 1 second before checking again")
		if i < maxAttempts-1 {
			time.Sleep(1 * time.Second) // Sleep for 1 second before retrying
		}
	}
	return fmt.Errorf("already waited for %d seconds, some services have still not stopped", maxAttempts)
}

// StartToolsAndServices starts the process for tools and services.
func StartToolsAndServices() {
	PrintBlue("Starting tools primarily involves component verification and other preparatory tasks.")
	if err := StartTools(); err != nil {
		PrintRed("Some tools failed to start, details are as follows, abort start")
		PrintRedNoTimeStamp(err.Error())
		return
	}
	PrintGreen("All tools executed successfully")

	KillExistBinaries()
	err := attemptCheckBinaries()
	if err != nil {
		PrintRed("Some services running, details are as follows, abort start " + err.Error())
		return
	}
	PrintBlue("Starting services involves multiple RPCs and APIs and may take some time. Please be patient")
	err = StartBinaries()
	if err != nil {
		PrintRed("Failed to start all binaries")
		PrintRedNoTimeStamp(err.Error())
		return
	}
	CheckAndReportBinariesStatus()
}

// CompileForPlatform Main compile function
func CompileForPlatform(platform string, compileBinaries []string) {

	PrintBlue(fmt.Sprintf("Compiling for platform: %s...", platform))

	var cmdBinaries, toolsBinaries []string

	for _, binary := range compileBinaries {
		if isCmdBinary(binary) {
			cmdBinaries = append(cmdBinaries, binary)
		} else if isToolBinary(binary) {
			toolsBinaries = append(toolsBinaries, binary)
		} else {
			PrintYellow(fmt.Sprintf("Binary %s not found in cmd or tools directories. Skipping...", binary))
		}
	}

	var cmdCompiledDirs []string
	var toolsCompiledDirs []string

	if len(cmdBinaries) > 0 {
		PrintBlue(fmt.Sprintf("Compiling cmd binaries for %s...", platform))
		cmdCompiledDirs = compileDir(filepath.Join(rootDirPath, "cmd"), platformsOutputBase, platform, cmdBinaries)
	}

	if len(toolsBinaries) > 0 {
		PrintBlue(fmt.Sprintf("Compiling tools binaries for %s...", platform))
		toolsCompiledDirs = compileDir(filepath.Join(rootDirPath, "tools"), toolsOutputBase, platform, toolsBinaries)
	}

	createStartConfigYML(cmdCompiledDirs, toolsCompiledDirs)
}

func createStartConfigYML(cmdDirs, toolsDirs []string) {
	configPath := filepath.Join(rootDirPath, "start-config.yml")

	if _, err := os.Stat(configPath); !os.IsNotExist(err) {
		PrintBlue("start-config.yml already exists, skipping creation.")
		return
	}

	var content strings.Builder
	content.WriteString("serviceBinaries:\n")
	for _, dir := range cmdDirs {
		content.WriteString(fmt.Sprintf("  %s: 1\n", dir))
	}
	content.WriteString("toolBinaries:\n")
	for _, dir := range toolsDirs {
		content.WriteString(fmt.Sprintf("  - %s\n", dir))
	}
	content.WriteString("maxFileDescriptors: 10000\n")

	err := os.WriteFile(configPath, []byte(content.String()), 0644)
	if err != nil {
		PrintRed("Failed to create start-config.yml: " + err.Error())
		return
	}
	PrintGreen("start-config.yml created successfully.")
}

func compileDir(sourceDir, outputBase, platform string, compileBinaries []string) []string {
	if info, err := os.Stat(sourceDir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		fmt.Printf("Failed read directory %s: %v\n", sourceDir, err)
		os.Exit(1)
	} else if !info.IsDir() {
		fmt.Printf("Failed %s is not dir\n", sourceDir)
		os.Exit(1)
	}

	var compiledDirs []string
	var mu sync.Mutex
	targetOS, targetArch := strings.Split(platform, "_")[0], strings.Split(platform, "_")[1]
	outputDir := filepath.Join(outputBase, targetOS, targetArch)

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		fmt.Printf("Failed to create directory %s: %v\n", outputDir, err)
		os.Exit(1)
	}

	var wg sync.WaitGroup
	errors := make(chan error, 1)
	sem := make(chan struct{}, 4)

	binSet := make(map[string]struct{})
	for _, bin := range compileBinaries {
		binSet[bin] = struct{}{}
	}

	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Base(path) != "main.go" {
			return nil
		}

		dir := filepath.Dir(path)
		dirName := filepath.Base(dir)

		if len(binSet) > 0 {
			if _, exists := binSet[dirName]; !exists {
				return nil
			}
		}

		wg.Add(1)
		go func(dir, dirName string) {
			sem <- struct{}{}
			defer wg.Done()
			defer func() { <-sem }()

			outputFileName := dirName
			if targetOS == "windows" {
				outputFileName += ".exe"
			}

			outputPath := filepath.Join(outputDir, outputFileName)

			PrintBlue(fmt.Sprintf("Compiling dir: %s for platform: %s binary: %s ...", dirName, platform, outputFileName))
			err := sh.RunWith(map[string]string{
				"GOOS":   targetOS,
				"GOARCH": targetArch,
			},
				"go", "build", "-o", outputPath, filepath.Join(dir, "main.go"))
			if err != nil {
				errors <- fmt.Errorf("failed to compile %s for %s: %v", dirName, platform, err)
				PrintRed("Compilation aborted. " + fmt.Sprintf("failed to compile %s for %s: %v", dirName, platform, err))
				os.Exit(1)
				return
			}
			PrintGreen(fmt.Sprintf("Successfully compiled. dir: %s for platform: %s binary: %s", dirName, platform, outputFileName))
			mu.Lock()
			compiledDirs = append(compiledDirs, dirName)
			mu.Unlock()
		}(dir, dirName)

		return nil
	})

	if err != nil {
		fmt.Println("Error walking through directories:", err)
		os.Exit(1)
	}

	wg.Wait()
	close(errors)

	// Check for errors
	if err, ok := <-errors; ok {
		fmt.Println(err)
		os.Exit(1)
	}

	return compiledDirs
}

func Build(binaries []string) {
	if _, err := os.Stat("start-config.yml"); err == nil {
		InitForSSC()
		KillExistBinaries()
	}

	InitForSSC()
	platforms := os.Getenv("PLATFORMS")
	if platforms == "" {
		platforms = DetectPlatform()
	}
	for _, platform := range strings.Split(platforms, " ") {
		compileBinaries := binaries
		if len(compileBinaries) == 0 {
			// build all
			compileBinaries = getAllBinaries()
		}
		CompileForPlatform(platform, compileBinaries)
	}
	PrintGreen("All specified binaries under cmd and tools were successfully compiled.")
}

func getAllBinaries() []string {
	cmdDirs, _ := filepath.Glob(filepath.Join(rootDirPath, "cmd", "*"))
	toolsDirs, _ := filepath.Glob(filepath.Join(rootDirPath, "tools", "*"))

	var allBinaries []string
	for _, dir := range cmdDirs {
		allBinaries = append(allBinaries, filepath.Base(dir))
	}

	for _, dir := range toolsDirs {
		allBinaries = append(allBinaries, filepath.Base(dir))
	}

	return allBinaries
}

func isCmdBinary(binary string) bool {
	path := filepath.Join(rootDirPath, "cmd", binary)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isToolBinary(binary string) bool {
	path := filepath.Join(rootDirPath, "tools", binary)
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
