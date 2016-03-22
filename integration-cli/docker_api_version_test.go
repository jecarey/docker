package main

import (
	"encoding/json"
	"fmt"
	"github.com/docker/docker/pkg/integration/checker"
	"github.com/go-check/check"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
)

func getDirContent(c *check.C, dir string, incFiles bool, incDirs bool) []string { // Com builder/dockerfile/parsr/parser_test.go?
	f, err := os.Open(dir)
	c.Assert(err, checker.IsNil)
	defer f.Close()

	dirEntries, err := f.Readdirnames(0)
	c.Assert(err, checker.IsNil)

	var contents []string

	for _, dirEntry := range dirEntries {
		f, err := os.Open(dir + "/" + dirEntry)
		c.Assert(err, checker.IsNil)
		defer f.Close()

		fstat, err := f.Stat()
		c.Assert(err, checker.IsNil)

		switch mode := fstat.Mode(); {
		case mode.IsDir():
			if incDirs {
				contents = append(contents, dirEntry)
			}
		case mode.IsRegular():
			if incFiles {
				contents = append(contents, dirEntry)
			}
		}
	}
	return contents
}

const CONTROLID string = ">>>CONTROL:"

func JsonContains(bigJson interface{}, smallJson interface{}, kvs map[string]interface{}) error {
	fmt.Printf("Comparing: \n\t%+v contains \n\t%+v\n", bigJson, smallJson)

	mask := false
	// Check for control in contained
	switch v := smallJson.(type) {
	case string:
		if strings.Contains(v, CONTROLID) {
			rawCntrl := strings.TrimPrefix(v, CONTROLID)
			rawCntrls := strings.Split(rawCntrl, ",")
			for _, rawCntrl := range rawCntrls {
				// fmt.Printf("  Control = %v\n", rawCntrl)
				rawCntrl = strings.TrimSpace(rawCntrl)
				var cntrlParm string
				if strings.Contains(rawCntrl, "(") {
					// NOTE(JEC): Assumes only one parameter
					tmpCntrl := strings.Split(rawCntrl, "(")
					rawCntrl = tmpCntrl[0]
					cntrlParm = strings.TrimSuffix(tmpCntrl[1], ")")
					// fmt.Printf("Contains parameter %v\n", cntrlParm)
				}
				switch rawCntrl {
				case "mask":
					mask = true
				case "store":
					mask = true // store implies mask since nothing to compare
					kvs[cntrlParm] = bigJson
					fmt.Printf("     Storing %v with key %v\n", bigJson, cntrlParm)
				case "replace":
					smallJson = kvs[cntrlParm]
					fmt.Printf("     replacing with %v using key %v\n", kvs[cntrlParm], cntrlParm)
				}
			}
		}
	}

	if mask {
		fmt.Printf("      ** Masked! **\n")
		return nil
	}

	if reflect.TypeOf(bigJson) != reflect.TypeOf(smallJson) {
		return fmt.Errorf("Kind does not match %v <> %v", reflect.TypeOf(bigJson), reflect.TypeOf(smallJson))
	}

	switch smallJsonA := smallJson.(type) {
	case map[string]interface{}:
		bigJsonA := bigJson.(map[string]interface{})

		if len(bigJsonA) > len(smallJsonA) {
			return fmt.Errorf("Not enough entries in map.  %v > %v", len(bigJsonA), len(smallJsonA))
		}

		for k, smallVal := range smallJsonA {
			bigVal, ok := bigJsonA[k]
			if !ok {
				return fmt.Errorf("Does not contain key: %v", k)
			}
			err := JsonContains(bigVal, smallVal, kvs)
			if err != nil {
				return err
			}
		}

	case []interface{}:
		bigJsonA := bigJson.([]interface{})
		// TODO: OK to have less if ones there match?
		if len(smallJsonA) != len(bigJsonA) {
			return fmt.Errorf("Not the same number of list elements in list: %v <> %v", len(bigJsonA), len(smallJsonA))
		}

		for i, smallVal := range smallJsonA {
			// check for cntrl
			err := JsonContains(bigJsonA[i], smallVal, kvs)
			if err != nil {
				return err
			}
		}

	default:
		if bigJson != smallJson {
			return fmt.Errorf("Value does not match: %v <> %v", bigJson, smallJson)
		}
	}
	return nil
}

func processControlFile(c *check.C, name string, version string) {
	f, err := ioutil.ReadFile(name)
	c.Assert(err, checker.IsNil)

	var testDesc []map[string]interface{}

	err = json.Unmarshal(f, &testDesc)
	c.Assert(err, checker.IsNil)
	// fmt.Printf("Results: %v\n", testDesc)

	kvs := map[string]interface{}{}
	for _, testDef := range testDesc {

		testMethod := testDef["method"].(string)
		testCommand := "/" + version + testDef["command"].(string)
		testRequestData := testDef["request"].(map[string]interface{})["data"]
		testResponseData := testDef["response"].(map[string]interface{})["data"]
		fmt.Printf("   Entry:\n")
		fmt.Printf("       Method: %v\n", testMethod)
		fmt.Printf("       Command: %v\n", testCommand)
		fmt.Printf("       Request: %v\n", testRequestData)
		fmt.Printf("       Response: %v\n", testResponseData)

		// TODO: Support replace from kvs into testCommand

		status, actualRespRaw, err := sockRequest(testMethod, testCommand, testRequestData)
		c.Assert(err, checker.IsNil)

		fmt.Printf("status = %v\n", status) // TODO check status matches
		//fmt.Printf("       Actual Response: %v\n", resp)

		switch expectedResponse := testResponseData.(type) { // Can't unmarshal a string
		case string: // Note(jec) This does not support control
			fmt.Printf("       Actual Response (string): %v\n", string(actualRespRaw))
			c.Assert(strings.TrimSpace(string(actualRespRaw)), checker.Contains, expectedResponse)
		default:
			var actualResponse interface{}
			err = json.Unmarshal(actualRespRaw, &actualResponse)
			c.Assert(err, checker.IsNil)
			//fmt.Printf("       Actual Response (default): %v\n", actualResponse)
			err = JsonContains(actualResponse, testResponseData, kvs)
			//fmt.Printf("       kvs = %v\n", kvs)
			c.Assert(err, checker.IsNil)
		}
	}
}

func (s *DockerSuite) TestApiVersion(c *check.C) {
	for _, dirName := range getDirContent(c, "./docker-api-version-data", false, true) {
		fmt.Println("Directory: " + dirName)

		newDirName := "./docker-api-version-data/" + dirName
		for _, fileName := range getDirContent(c, newDirName, true, false) {
			fmt.Println("test control:" + fileName)
			processControlFile(c, newDirName+"/"+fileName, dirName)
		}
	}
	c.Fatal("failing so I can see output")
}
