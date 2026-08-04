package main

import (
	"fmt"
	// "marveldigital/climgt" // [Commented out] climgt not available for local build
	"marveldigital/tag-reader-server/server"
	"os"

	"github.com/urfave/cli/v2"
)

// Version number
const Version = "0.3.2"

// Version flag
var GoVersion, BuildTime, GitCommit string

const appName = "Artefact Tag authenticate server"

const appUsage = "The server of Artefact NFC424DNA tag verify and data keeper"

// MakeCliApp create the basic Cli interface for config program
// [Commented out] climgt not available for local build
// func MakeCliApp(name string, usage string) (mgt *climgt.CliMgt) {
// 	server.VersionInfo = PrintBuildInfo()
// 	mgt = climgt.NewCliApp(name, usage, Version, server.ServerMain)
// 	if mgt == nil {
// 		return
// 	}
// 	mgt.AddFlag(&cli.UintFlag{
// 		Name:  "port",
// 		Value: 4430,
// 		Usage: "The server port",
// 	})
// 	mgt.AddFlag(&cli.BoolFlag{
// 		Name:  "https",
// 		Value: false,
// 		Usage: "Config server as HTTPS",
// 	})
// 	mgt.AddFlag(&cli.StringFlag{
// 		Name:  "cert",
// 		Value: "",
// 		Usage: "Select TLS server cert, if not provide use default",
// 	})
// 	mgt.AddFlag(&cli.StringFlag{
// 		Name:  "key",
// 		Value: "",
// 		Usage: "Select TLS server key",
// 	})
// 	mgt.AddFlag(&cli.StringFlag{
// 		Name:  "ip",
// 		Value: "",
// 		Usage: "Config the target ip server need to listen",
// 	})
// 	mgt.AddFlag(&cli.StringFlag{
// 		Name:  "domain",
// 		Value: "",
// 		Usage: "Config the domain name for public website access",
// 	})
// 	mgt.AddFlag(&cli.BoolFlag{
// 		Name:  "index",
// 		Value: false,
// 		Usage: "Set server have index redirect",
// 	})
// 	mgt.AddCommand(&cli.Command{
// 		Name:    "info",
// 		Aliases: []string{"i"},
// 		Usage:   "Show build info",
// 		Action: func(c *cli.Context) error {
// 			fmt.Print(PrintBuildInfo())
// 			return nil
// 		},
// 	})
// 	return
// }

func PrintBuildInfo() string {
	return fmt.Sprintf("\n Version: %s\n Go version: %s\n Build time: %s\n Git Commit: %s\n", Version, GoVersion, BuildTime, GitCommit)
}

func main() {
	server.VersionInfo = PrintBuildInfo()

	app := &cli.App{
		Name:  appName,
		Usage: appUsage,
		Action: func(c *cli.Context) error {
			return server.ServerMain(c)
		},
		Flags: []cli.Flag{
			&cli.UintFlag{
				Name:  "port",
				Value: 4430,
				Usage: "The server port",
			},
			&cli.BoolFlag{
				Name:  "https",
				Value: false,
				Usage: "Config server as HTTPS",
			},
			&cli.StringFlag{
				Name:  "cert",
				Value: "",
				Usage: "Select TLS server cert, if not provide use default",
			},
			&cli.StringFlag{
				Name:  "key",
				Value: "",
				Usage: "Select TLS server key",
			},
			&cli.StringFlag{
				Name:  "ip",
				Value: "",
				Usage: "Config the target ip server need to listen",
			},
			&cli.StringFlag{
				Name:  "domain",
				Value: "",
				Usage: "Config the domain name for public website access",
			},
			&cli.BoolFlag{
				Name:  "index",
				Value: false,
				Usage: "Set server have index redirect",
			},
		},
		Commands: []*cli.Command{
			{
				Name:    "info",
				Aliases: []string{"i"},
				Usage:   "Show build info",
				Action: func(c *cli.Context) error {
					fmt.Print(PrintBuildInfo())
					return nil
				},
			},
		},
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("server is closed")

	// [Commented out] climgt not available for local build
	// cliMgt := MakeCliApp(appName, appUsage)
	// defer cliMgt.CloseLog()
	//
	// err := cliMgt.Run(os.Args)
	// if err != nil {
	// 	climgt.Logio.Fatalln(err)
	// }
	// climgt.Logio.Printf("server is closed")
}
