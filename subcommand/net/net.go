package net

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BixData/binaryxml"
	"github.com/docopt/docopt-go"
	"github.com/spf13/viper"
)

const (
	FORMAT            = "format"
	FORMAT_BINARY_XML = "binaryxml"
	FORMAT_HEX        = "hex"
	FORMAT_STRING     = "string"
)

type Tee struct {
	Connection net.Conn
	File       *os.File
	Id         string
	PassThru   bool
}

func horizontalRule(title string) string {
	now := time.Now().String()
	newTitle := fmt.Sprintf("%s %s", now, title)
	result := "-------- " + newTitle + " " + strings.Repeat("-", 68-len(newTitle))
	return result
}

// Load configuration file.
func loadConfig(args map[string]interface{}) {

	// Set configuration file path.

	viper.SetConfigName("go-proxy-tee") // name of config file (without extension)

	// Add paths of where the configuration file may be found. Order is important.  First defined; first used.

	// Command-line option takes top precedence.

	configPathParameter := args["--configPath"]
	if configPathParameter != nil {
		viper.AddConfigPath(configPathParameter.(string))
	}

	// Other paths in precedence order.

	viper.AddConfigPath(".")
	viper.AddConfigPath("$HOME/go/src/github.com/docktermj/go-proxy-tee/")
	viper.AddConfigPath("$HOME/.go-proxy-tee") // call multiple times to add many search paths
	viper.AddConfigPath("/etc/go-proxy-tee/")  // path to look for the config file in

	// Load configuration contents.

	err := viper.ReadInConfig() // Find and read the config file
	if err != nil {             // Handle errors reading the config file
		panic(fmt.Errorf("Fatal error config file: %s \n", err))
	}

	// Command-line options override configuration file.

	debugParameter := args["--debug"]
	if debugParameter.(bool) {
		viper.Set("debug", true)
	}

	formatParameter := args["--format"]
	if formatParameter != nil {
		var format string
		switch strings.ToLower(formatParameter.(string)) {
		case FORMAT_STRING:
			format = FORMAT_STRING
		case FORMAT_HEX:
			format = FORMAT_HEX
		case FORMAT_BINARY_XML:
			format = FORMAT_BINARY_XML
		default:
			format = FORMAT_STRING
		}
		viper.Set(FORMAT, format)
	}

}

// One-way proxy from inbound (tee) to outbound.
// 'prefix' and network message are written to 'outFile'.
func proxy(ctx context.Context, tee Tee, outbound net.Conn, prefix string) {
	byteBuffer := make([]byte, 4096)

	// Read-write loop.

	for {

		// Read the inbound network connection.

		numberOfBytesRead, err := tee.Connection.Read(byteBuffer)
		if err != nil {
			log.Println("Proxy Read return")
			return
		}
		message := byteBuffer[0:numberOfBytesRead]

		// Construct output string for logging.

		var outString string
		switch viper.Get(FORMAT) {
		case FORMAT_STRING:
			outString = string(message)
		case FORMAT_HEX:
			outString = hex.Dump(message)
		case FORMAT_BINARY_XML:
			reader := bytes.NewReader(message)
			var param uint8
			xmlBuffer := make([]byte, 4096)
			err := binaryxml.ReadMessage(reader, &param, &xmlBuffer)
			if err != nil {
				fmt.Printf("binaryxml.ReadMessage() failed. Err: %+v\n", err)
			}
			outString, err = binaryxml.ToXML(xmlBuffer)
			if err != nil {
				fmt.Printf("binaryxml.ToXML() failed. Err: %+v\n", err)
			}
		default:
			outString = string(message)
		}

		// Log message to file.

		outline := fmt.Sprintf("%s\n%s\n\n", horizontalRule(prefix), outString)
		_, _ = tee.File.WriteString(outline)

		// If PassThru, write to outbound network connection.

		if tee.PassThru {
			_, err := outbound.Write([]byte(message))
			if err != nil {
				log.Println("Proxy Write return")
				return
			}
		}
	}
}

// One-way proxy from inbound to multiple outbounds via 'tees'
func proxyTee(ctx context.Context, inbound net.Conn, tees []Tee, prefix string) {
	byteBuffer := make([]byte, 4096)

	// Read-write loop.

	for {

		// Read the inbound network connection.

		numberOfBytesRead, err := inbound.Read(byteBuffer)
		if err != nil {
			log.Println("Proxy Read return")
			return
		}
		message := byteBuffer[0:numberOfBytesRead]

		// Construct output string for logging.

		var outString string
		switch viper.Get(FORMAT) {
		case FORMAT_STRING:
			outString = string(message)
		case FORMAT_HEX:
			outString = hex.Dump(message)
		case FORMAT_BINARY_XML:
			reader := bytes.NewReader(message)
			var param uint8
			xmlBuffer := make([]byte, 4096)
			err := binaryxml.ReadMessage(reader, &param, &xmlBuffer)
			if err != nil {
				fmt.Printf("binaryxml.ReadMessage() failed. Err: %+v\n", err)
			}
			outString, err = binaryxml.ToXML(xmlBuffer)
			if err != nil {
				fmt.Printf("binaryxml.ToXML() failed. Err: %+v\n", err)
			}
		default:
			outString = string(message)
		}

		// Construct the message for logging.

		outline := fmt.Sprintf("%s\n%s\n\n", horizontalRule(prefix), outString)

		// Process each tee as outbound.

		for _, tee := range tees {

			// Log message to tee's file.

			_, _ = tee.File.WriteString(outline)

			// Write to tee's outbound network connection.

			_, err := tee.Connection.Write([]byte(message))
			if err != nil {
				log.Println("Proxy Write return")
				return
			}
		}
	}
}

// Function for the "command pattern".
func Command(argv []string) {

	usage := `
Usage:
    go-proxy-tee net [options]

Options:
   -h, --help
   --configPath=<configuration_path>   Directory of go-proxy-tee.json configuration file
   --format=<format>                   Output format.
   --debug                             Log debugging messages

Where:
   configuration_path   Example: '/path/to/configuration'
   format               Example: 'string', 'hex', 'binaryxml'  Default: string.
`

	// DocOpt processing.

	args, _ := docopt.Parse(usage, nil, true, "", false)

	// Get configuration.

	loadConfig(args)
	inboundNetwork := viper.GetString("inbound.network")
	inboundAddress := viper.GetString("inbound.address")
	outboundNetwork := viper.GetString("outbound.network")
	outboundAddress := viper.GetString("outbound.address")
	outboundOutput := viper.GetString("outbound.output")
	isDebug := viper.GetBool("debug")
	teeDefinitions := viper.GetStringMap("tee")

	// Debugging information.

	if isDebug {
		log.Printf("Listening on '%s' network with address '%s'\n", inboundNetwork, inboundAddress)
		log.Printf("Communicating with '%s' network with address '%s' into file '%s'\n", outboundNetwork, outboundAddress, outboundOutput)
		teeDefinitions := viper.GetStringMap("tee")
		for key, _ := range teeDefinitions {
			teeDefinition := teeDefinitions[key].(map[string]interface{})
			teeNetwork := teeDefinition["network"].(string)
			teeAddress := teeDefinition["address"].(string)
			teeOutput := teeDefinition["output"].(string)
			log.Printf("Tee-ing to '%s' network with address '%s' into file '%s'\n", teeNetwork, teeAddress, teeOutput)
		}
		log.Printf("Formatting output as '%s'\n", viper.GetString(FORMAT))
	}

	// Create context.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Inbound listener.  net.Listen creates a server.

	inboundListener, err := net.Listen(inboundNetwork, inboundAddress)
	if err != nil {
		log.Fatal("Listen error: ", err)
	}

	// Configure listener to exit when program ends.

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func(listener net.Listener, c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.", sig)
		listener.Close()
		os.Exit(0)
	}(inboundListener, sigc)

	// As a server, Read and Echo loop.

	for {
		tees := []Tee{}

		// As a server, listen for a connection request.

		inboundConnection, err := inboundListener.Accept()
		if err != nil {
			log.Fatal("Accept error: ", err)
		}
		if isDebug {
			log.Println("Accepted inbound connection.")
		}

		// Create output file.

		outboundFile, err := os.OpenFile(outboundOutput, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
		if err != nil {
			panic(err)
		}
		defer outboundFile.Close()

		// Create a network connection.  net.Dial creates a client.

		outboundConnection, err := net.Dial(outboundNetwork, outboundAddress)
		if err != nil {
			log.Fatal("Dial error", err)
		}
		defer outboundConnection.Close()

		// Add "outbound" to tees with PassThru=true.

		tee := Tee{
			Connection: outboundConnection,
			File:       outboundFile,
			Id:         "outbound",
			PassThru:   true,
		}
		tees = append(tees, tee)

		// Add tees from configuration file.

		for key, _ := range teeDefinitions {

			// Get configuration.

			teeDefinition := teeDefinitions[key].(map[string]interface{})
			teeNetwork := teeDefinition["network"].(string)
			teeAddress := teeDefinition["address"].(string)
			teeOutput := teeDefinition["output"].(string)

			// Create output file.

			teeFile, err := os.OpenFile(teeOutput, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
			if err != nil {
				panic(err)
			}
			defer teeFile.Close()

			// Create a network connection.  net.Dial creates a client.

			teeConnection, err := net.Dial(teeNetwork, teeAddress)
			if err != nil {
				log.Fatal("Dial error", err)
			}
			defer teeConnection.Close()

			// Add to tees.

			tee := Tee{
				Connection: teeConnection,
				File:       teeFile,
				Id:         key,
			}
			tees = append(tees, tee)
		}

		// Asynchronously handle bi-directional traffic.

		go proxyTee(ctx, inboundConnection, tees, "Client request")
		for _, tee := range tees {
			go proxy(ctx, tee, inboundConnection, "Server response")
		}
	}
}
