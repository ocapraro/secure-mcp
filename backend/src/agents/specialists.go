package agents

import (
	"encoding/xml"
	"log"
	"os"
	"path/filepath"
)

type ScriptArgument struct {
	Name     string `xml:"name,attr"`
	Required string `xml:"required,attr"`
	Value    string `xml:",chardata"`
}

type ScriptSecret struct {
	Name     string `xml:"name,attr"`
	Required string `xml:"required,attr"`
	Value    string `xml:",chardata"`
}

type Plugin struct {
	Path        string           `xml:"path"`
	Description string           `xml:"description"`
	Usage       string           `xml:"usage"`
	Arguments   []ScriptArgument `xml:"arguments>argument"`
	Secrets     []ScriptSecret   `xml:"secrets>secret"`
	Output      string           `xml:"output"`
	Example     string           `xml:"example"`
}

type Specialist struct {
	Name           string   `xml:"name"`
	Version        string   `xml:"version"`
	Resume         string   `xml:"resume"`
	ExampleRequest string   `xml:"exampleRequest"`
	Plugins        []Plugin `xml:"plugins>script"`
	Directory      string   `xml:"-"`
}

type specialistXML struct {
	XMLName xml.Name `xml:"specialist"`
	Specialist
}

const specialistsDir = "../specialists"

func ListSpecialists() []Specialist {
	entries, err := os.ReadDir(specialistsDir)
	if err != nil {
		log.Printf("warning: could not read specialists directory: %v", err)
		return nil
	}

	var specialists []Specialist
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		bioPath := filepath.Join(specialistsDir, entry.Name(), "bio.xml")
		data, err := os.ReadFile(bioPath)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			log.Printf("warning: skipping %s: %v", bioPath, err)
			continue
		}
		var s specialistXML
		if err := xml.Unmarshal(data, &s); err != nil {
			log.Printf("warning: failed to parse %s: %v", bioPath, err)
			continue
		}
		s.Specialist.Directory = entry.Name()
		specialists = append(specialists, s.Specialist)
	}
	return specialists
}
