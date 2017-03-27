package util

import (
	"io"
	"io/ioutil"

	"github.com/pkg/errors"

	"gopkg.in/yaml.v2"
)

// ReadYAMLInto reads data for the given io.ReadCloser - until it hits an error
// or reaches EOF - and attempts to unmarshal the data read into the given
// interface.
func ReadYAMLInto(r io.ReadCloser, data interface{}) error {
	defer r.Close()
	bytes, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(bytes, data)
}

// UnmarshalYAMLFile reads in the specified file, and unmarshals it
// into the given interface. Returns an error if one is encountered
// in reading the file or if the file does not contain valid YAML.
func UnmarshalYAMLFile(file string, data interface{}) error {
	fileBytes, err := ioutil.ReadFile(file)
	if err != nil {
		return errors.Errorf("error reading file %v: %v", file, err)
	}
	if err = yaml.Unmarshal(fileBytes, data); err != nil {
		return errors.Errorf("error unmarshalling yaml from %v: %v", file, err)
	}
	return nil
}
