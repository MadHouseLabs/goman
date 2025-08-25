package main

import (
	"fmt"
	"gopkg.in/yaml.v2"
)

func main() {
	yamlContent := `
description: "Development cluster"
region: ap-south-1
instanceType: t3.medium

nodePools: []
`

	var config map[string]interface{}
	if err := yaml.Unmarshal([]byte(yamlContent), &config); err != nil {
		fmt.Printf("Error unmarshaling: %v\n", err)
		return
	}

	fmt.Printf("Parsed config: %+v\n", config)
	
	if nodePoolsRaw, ok := config["nodePools"]; ok {
		fmt.Printf("nodePools type: %T\n", nodePoolsRaw)
		fmt.Printf("nodePools value: %+v\n", nodePoolsRaw)
		
		if npList, ok := nodePoolsRaw.([]interface{}); ok {
			fmt.Printf("Successfully cast to []interface{}, length: %d\n", len(npList))
			for i, np := range npList {
				fmt.Printf("NodePool %d type: %T\n", i, np)
				fmt.Printf("NodePool %d value: %+v\n", i, np)
			}
		} else {
			fmt.Println("Failed to cast to []interface{}")
		}
	} else {
		fmt.Println("No nodePools found")
	}
}