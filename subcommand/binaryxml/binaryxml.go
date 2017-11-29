package binaryxml

import (
	"bytes"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/BixData/binaryxml"
	"github.com/BixData/binaryxml/messages"
	"github.com/docopt/docopt-go"
	"github.com/spf13/viper"
)

const (
	BINARY_XML_START uint8 = 121
)

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
}

// Pretty-print XML.
func formatXml(data []byte) ([]byte, error) {
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

func formatBinaryXml(inputFileName string) {
	var param uint8
	xmlBuffer := make([]byte, 4096)

	// Create input bytes Reader for inputFileName.

	inputFile, err := os.Open(inputFileName)
	if err != nil {
		panic(err)
	}
	defer inputFile.Close()
	inputFileBytes, err := ioutil.ReadAll(inputFile)
	if err != nil {
		panic(err)
	}
	reader := bytes.NewReader(inputFileBytes)

	// Create output.

	outputFileName := fmt.Sprintf("%s.xml", inputFileName)
	outputFile, err := os.OpenFile(outputFileName, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		panic(err)
	}
	defer outputFile.Close()

	maxReaderLength := reader.Len()
	for reader.Len() > 0 {
		currentOffset := maxReaderLength - reader.Len()
		//		fmt.Printf(">>> Offset: %X\n", currentOffset)

		err := binaryxml_messages.ReadMessage(reader, &param, &xmlBuffer)
		if err != nil {
			fmt.Printf("binaryxml.ReadMessage() failed. Err: %+v\n", err)
			badOffset := currentOffset - 1

			aByte := make([]byte, 1)
			reader.Seek(int64(badOffset+1), 0) //  0 means from begining of file. https://socketloop.com/references/golang-bytes-reader-seek-function-example
			binaryXmlStart := []byte{BINARY_XML_START}
			for bytes.Compare(aByte, binaryXmlStart) != 0 {
				_, err := reader.Read(aByte)
				if err != nil {
					break
				}
			}
			currentOffset := maxReaderLength - reader.Len()

			message := hex.Dump(inputFileBytes[badOffset : currentOffset-1])
			fmt.Printf("Offset %X:\n%s\n", badOffset, message)
		}
		binaryXmlString, err := binaryxml.ToXML(xmlBuffer)
		if err != nil {
			fmt.Printf("binaryxml.ToXML() failed. Err: %+v\n", err)
		}
		if len(binaryXmlString) > 0 {
			formattedXml, err := formatXml([]byte(binaryXmlString))
			if err != nil {
				panic(err)
			}
			_, err = outputFile.Write(formattedXml)
			if err != nil {
				panic(err)
			}
			_, err = outputFile.WriteString("\n")
			if err != nil {
				panic(err)
			}
		}
	}

}

// Function for the "command pattern".
func Command(argv []string) {

	usage := `
Usage:
    go-proxy-tee binaryxml [options]

Options:
   -h, --help
   --configPath=<configuration_path>   Directory of go-proxy-tee.json configuration file
   --debug                             Log debugging messages

Where:
   configuration_path   Example: '/path/to/configuration'
`

	// DocOpt processing.

	args, _ := docopt.Parse(usage, nil, true, "", false)

	// Get configuration.

	loadConfig(args)

	// Transform input, output, and tee files.

	//	inboundOutput := viper.GetString("inbound.output")
	//	formatBinaryXml(inboundOutput)

	outboundOutput := viper.GetString("outbound.output")
	formatBinaryXml(outboundOutput)

	teeDefinitions := viper.GetStringMap("tee")
	for key, _ := range teeDefinitions {
		teeDefinition := teeDefinitions[key].(map[string]interface{})
		teeOutput := teeDefinition["output"].(string)
		formatBinaryXml(teeOutput)
	}
}
