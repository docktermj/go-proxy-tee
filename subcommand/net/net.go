package net

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/BixData/binaryxml"
	"github.com/BixData/binaryxml/messages"
	"github.com/docopt/docopt-go"
	"github.com/spf13/viper"
)

const (

	// Lengths in XML.

	BINARY_XML_LENGTH_BEGIN_TOKEN = 1
	BINARY_XML_LENGTH_LENGTH      = 4
	BINARY_XML_LENGTH_PARAM       = 1
	BINARY_XML_LENGTH_END_TOKEN   = 1
	BINARY_XML_LENGTH_CRC         = 4

	BINARY_XML_LENGTHS = BINARY_XML_LENGTH_BEGIN_TOKEN +
		BINARY_XML_LENGTH_LENGTH +
		BINARY_XML_LENGTH_PARAM +
		BINARY_XML_LENGTH_END_TOKEN +
		BINARY_XML_LENGTH_CRC

	// Sentinals in XML.

	BINARY_XML_START        uint8 = 121
	BINARY_XML_TABLE_BEGIN  uint8 = 124
	BINARY_XML_TABLE_END    uint8 = 125
	BINARY_XML_SERIAL_BEGIN uint8 = 126
	BINARY_XML_SERIAL_END   uint8 = 127
	BINARY_XML_STOP         uint8 = 123

	// Acceptable output file formats.

	FORMAT             = "format"
	FORMAT_BINARY_FILE = "binaryfile"
	FORMAT_BINARY_XML  = "binaryxml"
	FORMAT_HEX         = "hex"
	FORMAT_HEX_PARSED  = "hexparsed"
	FORMAT_STRING      = "string"

	BUFFER_LENGTH = 1024 * 16
)

type Tee struct {
	Address    string
	Connection net.Conn
	File       *os.File
	Id         string
	Network    string
	Output     string
	PassThru   bool
}

type Inbound struct {
	Address    string
	Connection net.Conn
	File       *os.File
	Listener   net.Listener
	Network    string
	Output     string
}

// Make a timestampped "horizontal rule" to separate output into groups.
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

	// Other paths in precedence order.  Order is important.

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
		case FORMAT_BINARY_FILE:
			format = FORMAT_BINARY_FILE
		case FORMAT_BINARY_XML:
			format = FORMAT_BINARY_XML
		case FORMAT_HEX:
			format = FORMAT_HEX
		case FORMAT_HEX_PARSED:
			format = FORMAT_HEX_PARSED
		case FORMAT_STRING:
			format = FORMAT_STRING
		default:
			format = FORMAT_STRING
		}
		viper.Set(FORMAT, format)
	}
}

// Pretty-print XML.
func formatXML(data []byte) ([]byte, error) {
	b := &bytes.Buffer{}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	encoder := xml.NewEncoder(b)
	encoder.Indent("", "   ")
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			encoder.Flush()
			return b.Bytes(), nil
		}
		if err != nil {
			return nil, err
		}
		err = encoder.EncodeToken(token)
		if err != nil {
			return nil, err
		}
	}
}

func hexParseSplit(message []byte) []byte {

	reader := bytes.NewReader(message)
	splitLength := uint32(len(message))

	// Read token.

	var token uint8
	if err := binary.Read(reader, binary.BigEndian, &token); err != nil {
		return message
	}

	// Based on the token, determine how to find a split.

	switch token {
	case BINARY_XML_START:
		var messageLength uint32
		if err := binary.Read(reader, binary.BigEndian, &messageLength); err != nil {
			return message
		}
		finalLength := messageLength + BINARY_XML_LENGTHS
		if finalLength < splitLength {
			splitLength = finalLength
		}
	default:
		return message
	}

	return message[:splitLength]

}

func hexParse(message []byte) string {
	result := ""
	offset := 0
	for offset < len(message) {
		slice := hexParseSplit(message[offset:])
		result = fmt.Sprintf("%s\n%s", result, hex.Dump(slice))
		offset += len(slice)
	}
	return result
}

func binaryxmlParse(message []byte) string {
	result := hex.Dump(message)
	var param uint8
	xmlBuffer := make([]byte, BUFFER_LENGTH)
	offset := 0

	for offset < len(message) {
		switch message[offset] {
		case BINARY_XML_START:
			reader := bytes.NewReader(message[offset:])
			readerOriginalLength := reader.Len()
			err := messages.ReadMessage(reader, &param, &xmlBuffer)
			if err != nil {
				log.Printf("binaryxml_messages.ReadMessage() failed. Err: %+v\n", err)
				break
			}
			readerFinalLength := reader.Len()
			binaryXmlString, err := binaryxml.ToXML(xmlBuffer)
			if err != nil {
				log.Printf("binaryxml.ToXML() failed. Err: %+v\n", err)
				break
			}
			if len(binaryXmlString) > 0 {
				formattedXML, _ := formatXML([]byte(binaryXmlString))
				result = fmt.Sprintf("%s\n%s", result, formattedXML)
			}
			offset = offset + (readerOriginalLength - readerFinalLength)
		default:
			offset = len(message)
		}
	}
	return result
}

// Open a file for writing.
func openFile(ctx context.Context, fileName string) *os.File {
	file, err := os.OpenFile(fileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		panic(err)
	}
	return file
}

// Convenience method for "Inbound" object.
func openInputFile(ctx context.Context, inbound *Inbound) {
	inbound.File = openFile(ctx, inbound.Output)
}

// Convenience method for "Tee" object.
func openOutputFile(ctx context.Context, tee *Tee) {
	tee.File = openFile(ctx, tee.Output)
}

// As a server, listen on a port.
func listen(ctx context.Context, inbound *Inbound) {

	if inbound.Connection != nil {
		inbound.Connection.Close()
	}

	// Inbound listener.  net.Listen creates a server.

	inboundListener, err := net.Listen(inbound.Network, inbound.Address)
	if err != nil {
		log.Fatal("Listen error: ", err)
	}

	// Configure listener to exit when program ends.

	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, os.Interrupt, syscall.SIGTERM)
	go func(listener net.Listener, c chan os.Signal) {
		sig := <-c
		log.Printf("Caught signal %s: shutting down.\n", sig)
		listener.Close()
		os.Exit(0)
	}(inboundListener, sigc)

	inbound.Listener = inboundListener
}

// As a server, accept a connection request.
// This is a blocking function.   It waits until client makes a request.
func accept(ctx context.Context, inbound *Inbound) {
	isDebug := viper.GetBool("debug")

	inboundConnection, err := inbound.Listener.Accept()
	if err != nil {
		log.Fatalf("inbound.Listener.Accept() failed. Err: %+v\n", err)
	}
	if isDebug {
		log.Println("Accepted inbound connection.")
	}
	inbound.Connection = inboundConnection
}

// As a client, connect to a service.
func connect(ctx context.Context, tee *Tee) {
	if tee.Connection != nil {
		tee.Connection.Close()
	}
	teeConnection, err := net.Dial(tee.Network, tee.Address)
	if err != nil {
		log.Fatal("net.Dial error", err)
	}
	tee.Connection = teeConnection
}

// Append a Tee to a list of Tees.
// Also, open the output file and connect to service.
func appendTee(ctx context.Context, tees []Tee, tee Tee) []Tee {
	openOutputFile(ctx, &tee)
	connect(ctx, &tee)
	return append(tees, tee)
}

// One-way proxy from inbound (tee) to outbound.
// 'prefix' and network message are written to 'outFile'.
func proxy(ctx context.Context, tee Tee, outbound Inbound, prefix string) {
	isDebug := viper.GetBool("debug")
	byteBuffer := make([]byte, BUFFER_LENGTH)

	// Read-write loop.

	for {

		// Read the inbound network connection.

		numberOfBytesRead, err := tee.Connection.Read(byteBuffer)
		if err != nil {
			log.Printf("tee.Connection.Read(...) failed. Err: %+v\n", err)
			return
		}

		message := make([]byte, numberOfBytesRead)
		copy(message, byteBuffer[0:numberOfBytesRead])

		// Construct output string for logging.

		var outString string
		switch viper.Get(FORMAT) {
		case FORMAT_BINARY_FILE:
			outString = ""
		case FORMAT_BINARY_XML:
			outString = binaryxmlParse(message)
		case FORMAT_HEX:
			outString = hex.Dump(message)
		case FORMAT_HEX_PARSED:
			outString = hexParse(message)
		case FORMAT_STRING:
			outString = string(message)
		default:
			outString = string(message)
		}

		// Log message to file.

		if len(outString) > 0 {
			outline := fmt.Sprintf("%s\n%s\n\n", horizontalRule(prefix), outString)
			_, _ = tee.File.WriteString(outline)
		} else {
			_, _ = tee.File.Write(byteBuffer[0:numberOfBytesRead])
		}

		// If PassThru, write to outbound network connection.

		if tee.PassThru {
			if isDebug {
				log.Printf("Bytes returned by proxy: %d\n", numberOfBytesRead)
			}
			_, err := outbound.Connection.Write(byteBuffer[0:numberOfBytesRead])
			if err != nil {
				log.Printf("outbound.Write() failed. Err: %+v\n", err)
				return
			}
		}
	}
}

// One-way proxy from inbound to multiple outbounds via 'tees'
func proxyTee(ctx context.Context, inbound Inbound, tees []Tee, prefix string) {
	isDebug := viper.GetBool("debug")
	byteBuffer := make([]byte, BUFFER_LENGTH)

	// Read-write loop.

	for {

		// Read the inbound network connection.

		numberOfBytesRead, err := inbound.Connection.Read(byteBuffer)
		if err != nil {
			log.Printf("inbound.Connection.Read() failed. Err: %+v\n", err)
			return
		}

		if isDebug {
			log.Printf("Bytes sent to proxy: %d\n", numberOfBytesRead)
		}

		message := make([]byte, numberOfBytesRead)
		copy(message, byteBuffer[0:numberOfBytesRead])

		// Construct output string for logging.

		var outString string
		switch viper.Get(FORMAT) {
		case FORMAT_BINARY_FILE:
			outString = ""
			inbound.File.Write(message)
		case FORMAT_BINARY_XML:
			outString = binaryxmlParse(message)
		case FORMAT_HEX:
			outString = hex.Dump(message)
		case FORMAT_HEX_PARSED:
			outString = hexParse(message)
		case FORMAT_STRING:
			outString = string(message)
		default:
			outString = string(message)
		}

		// Construct the message for logging.

		outline := fmt.Sprintf("%s\n%s\n\n", horizontalRule(prefix), outString)

		// Process each tee as outbound.

		for _, tee := range tees {

			// Log message to tee's file.

			if len(outString) > 0 {
				_, _ = tee.File.WriteString(outline)
			}

			// Write to tee's outbound network connection.

			_, err := tee.Connection.Write(byteBuffer[0:numberOfBytesRead])
			if err != nil {
				log.Printf("tee.Connection.Write() failed. Err: %+v\n", err)
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
   format               Values: 'binaryfile', 'binaryxml', 'hex', 'hexparsed', and default value: 'string'.
`

	// Create context.

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// DocOpt processing.

	args, _ := docopt.Parse(usage, nil, true, "", false)

	// Get configuration.

	loadConfig(args)
	inboundNetwork := viper.GetString("inbound.network")
	inboundAddress := viper.GetString("inbound.address")
	inboundOutput := viper.GetString("inbound.output")
	outboundNetwork := viper.GetString("outbound.network")
	outboundAddress := viper.GetString("outbound.address")
	outboundOutput := viper.GetString("outbound.output")
	isDebug := viper.GetBool("debug")
	teeDefinitions := viper.GetStringMap("tee")

	// Debugging information.

	if isDebug {
		log.Printf("Listening on '%s' network with address '%s' into file '%s'\n", inboundNetwork, inboundAddress, inboundOutput)
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

	// Initialize inbound listener.

	inbound := Inbound{
		Address: inboundAddress,
		Network: inboundNetwork,
		Output:  inboundOutput,
	}
	listen(ctx, &inbound)
	openInputFile(ctx, &inbound)
	defer inbound.File.Close()

	// As a server, Read and Echo loop.

	for {
		tees := []Tee{}

		// As a server, listen for a connection request. This is blocking.

		accept(ctx, &inbound)

		// Create a "per-connection" context.

		connectionCtx, connectionCtxCancel := context.WithCancel(ctx)
		defer connectionCtxCancel()

		// Add "outbound" to tees with PassThru=true.

		tee := Tee{
			Address:  outboundAddress,
			Id:       "outbound",
			Network:  outboundNetwork,
			Output:   outboundOutput,
			PassThru: true,
		}
		tees = appendTee(connectionCtx, tees, tee)

		// Add tees from configuration file.

		for key, _ := range teeDefinitions {
			teeDefinition := teeDefinitions[key].(map[string]interface{})
			tee := Tee{
				Address: teeDefinition["address"].(string),
				Id:      key,
				Network: teeDefinition["network"].(string),
				Output:  teeDefinition["output"].(string),
			}
			tees = appendTee(connectionCtx, tees, tee)
		}

		// Asynchronously handle bi-directional traffic.

		defer inbound.Connection.Close()
		go proxyTee(connectionCtx, inbound, tees, "Client request")
		for _, tee := range tees {
			defer tee.Connection.Close()
			go proxy(connectionCtx, tee, inbound, "Server response")
		}
	}
}
