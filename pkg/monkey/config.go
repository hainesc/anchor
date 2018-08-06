/*
 * Copyright 2018 Haines Chan
 *
 * This program is free software; you can redistribute and/or modify it
 * under the terms of the standard MIT license. See LICENSE for more details
 */

package monkey

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
)

// CaptainConf is the config for Caption, Maybe we should rename it to MonkeyConf.
type CaptainConf struct {
	// etcd client
	Endpoints string `json:"etcd_endpoints"`
	// etcd perm files
	CertFile      string `json:"etcd_cert_file"`
	KeyFile       string `json:"etcd_key_file"`
	TrustedCAFile string `json:"etcd_ca_cert_file"`
}

// NotFoundError when the directory and file not found.
type NotFoundError struct {
	Dir  string
	Name string
}

// Error prints out the error message for NotFoundError
func (e NotFoundError) Error() string {
	return fmt.Sprintf(`no net configuration with name "%s" in %s`, e.Name, e.Dir)
}

// NoConfigsFoundError when no config files found in dir
type NoConfigsFoundError struct {
	Dir string
}

// Error print out the error message for NoConfigFoundError
func (e NoConfigsFoundError) Error() string {
	return fmt.Sprintf(`no net configurations found in %s`, e.Dir)
}

// LoadConf loads config for Monkey
func LoadConf(dir, name string) (*CaptainConf, error) {
	files, err := ConfFiles(dir, []string{".conf", ".json"})
	switch {
	case err != nil:
		return nil, err
	case len(files) == 0:
		return nil, NoConfigsFoundError{Dir: dir}
	}
	sort.Strings(files)

	for _, confFile := range files {
		conf, err := ConfFromFile(confFile)
		if err != nil {
			return nil, err
		}
		return conf, nil

	}
	return nil, NotFoundError{dir, name}

}

// ConfFromFile reads config from file
func ConfFromFile(filename string) (*CaptainConf, error) {
	bytes, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading %s: %s", filename, err)
	}
	return ConfFromBytes(bytes)
}

// ConfFromBytes reads config from byte array
func ConfFromBytes(bytes []byte) (*CaptainConf, error) {
	conf := &CaptainConf{}
	if err := json.Unmarshal(bytes, &conf); err != nil {
		return nil, fmt.Errorf("error parsing configuration: %s", err)
	}
	return conf, nil
}

// ConfFiles returns config file list in the dir which has specail extensions
func ConfFiles(dir string, extensions []string) ([]string, error) {
	files, err := ioutil.ReadDir(dir)
	switch {
	case err == nil: // break
	case os.IsNotExist(err):
		return nil, nil
	default:
		return nil, err
	}

	confFiles := []string{}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		fileExt := filepath.Ext(f.Name())
		for _, ext := range extensions {
			if fileExt == ext {
				confFiles = append(confFiles, filepath.Join(dir, f.Name()))
			}
		}
	}
	return confFiles, nil
}
