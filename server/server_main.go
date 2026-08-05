package server

import (
	"errors"
	"fmt"
	"io"
	"log"
	// "marveldigital/climgt" // [Commented out] climgt not available for local build
	ancientauth "marveldigital/tag-reader-server/ancientAuth"
	"os"
	"strconv"
	"strings"

	"github.com/kataras/iris/v12"
	"github.com/urfave/cli/v2"
)

var (
	dbPath   = "./local" // path for any database
	servPort = "4430"    // port for http(s) server
	hostname = ""        // deprecated, host name for cookie value
	servIP   string      // IP for http(s) server, internal

	logio *log.Logger

	appdebug  bool = false
	haveIndex      = false

	ErrUpdateLogger = errors.New("logger update failed")
)

const maxSize = 64 << 20 // 64MB

// The server log info
var VersionInfo string

func ServerMain(c *cli.Context) (err error) {
	defer func() {
		if err := recover(); err != nil {
			log.Printf("Error happen: %s", err)
		}
	}()

	var cert, certkey string
	var usehttps bool

	cert, certkey, usehttps, hostname = checkCli(c)

	app := newIrisApp()
	if app == nil {
		return errors.New("iris application create failed")
	}
	if usehttps {
		err = app.Run(iris.TLS(servIP+":"+servPort, cert, certkey), iris.WithPostMaxMemory(maxSize))
	} else {
		err = app.Run(iris.Addr(servIP+":"+servPort), iris.WithPostMaxMemory(maxSize))
	}
	defer Close()

	return
}

func UpdateLogio(logger *log.Logger) error {
	if logger != nil {
		logio = logger
		return nil
	}
	return ErrUpdateLogger
}

func SetDbPath(path string) {
	if p, err := os.Stat(path); err != nil && os.IsNotExist(err) {
		log.Panicf("DBPath error, %s", err)
	} else if !p.IsDir() {
		log.Panicf("DBpath error, path is not dir")
	}
	dbPath = path
}

func Close() {
	AppSess.Kill()
	if cardAdmin != nil {
		log.Printf("cardAdmin closed with %s", cardAdmin.CloseDB())
	}
	cardDB.Close()
}

func checkCli(c *cli.Context) (cert string, certkey string, usehttps bool, host string) {
	usehttps = false
	checkPath := func(args string) (str string) {
		f, err := os.Open(args)
		if err == nil {
			str = args
		}
		defer f.Close()
		return
	}

	checkPort := func(intport uint) (port string) {
		if intport > 0 && intport < 49152 {
			port = strconv.FormatUint(uint64(intport), 10)
			return
		}
		log.Printf("Error: port number is invalid")
		return
	}
	if port := checkPort(c.Uint("port")); port != "" {
		servPort = port
	}
	if path := checkPath(c.String("cert")); path != "" {
		cert = path
	}
	if path := checkPath(c.String("key")); path != "" {
		certkey = path
	}
	if ip := c.String("ip"); ip != "" {
		if addrs := strings.Split(ip, "."); len(addrs) == 4 {
			servIP = ip
			fmt.Printf("Set listen IP: %s", ip)
		}
	}
	usehttps = c.Bool("https")
	appdebug = c.Bool("debug")
	host = c.String("domain") // deprecated
	haveIndex = c.Bool("index")

	if cert == "" || certkey == "" {
		cert = "server.crt"
		certkey = "server-key.pem"
		log.Printf("Require cert pair file, one of file is missing, will use default cert & key")
	}
	// [Commented out] climgt not available for local build
	// if err := UpdateLogio(climgt.Logio); err != nil {
	// 	panic(err)
	// }
	// [Replacement] Use local logger writing to stdout instead of climgt.Logio
	if err := UpdateLogio(log.New(io.MultiWriter(os.Stdout), "", log.LstdFlags)); err != nil {
		panic(err)
	}
	logio.Print(VersionInfo)
	logio.Printf("Loaded the cli args, config:\n tls: %t\n debug: %t\n port: %s\n cert path: %s\n certkey path: %s",
		usehttps, appdebug, servPort, cert, certkey)
	return
}

func newIrisApp() (newapp *iris.Application) {
	newapp = iris.New()

	MakeAdminPage(newapp)
	MakeCardPage(newapp)
	ancientauth.MakeAncientAuthServer(newapp)

	return
}
