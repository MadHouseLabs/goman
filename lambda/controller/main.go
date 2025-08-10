package main

import (
	"fmt"
	"log"
	"os"
	
	"github.com/madhouselabs/goman/pkg/provider/aws"
)

func main() {
	// Log environment info for debugging
	log.Println("=== Lambda Main Starting ===")
	log.Printf("AWS Region: %s", os.Getenv("AWS_REGION"))
	log.Printf("Lambda Task Root: %s", os.Getenv("LAMBDA_TASK_ROOT"))
	log.Printf("Lambda Runtime API: %s", os.Getenv("AWS_LAMBDA_RUNTIME_API"))
	
	// Catch any panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("PANIC RECOVERED: %v", r)
			panic(r) // re-panic after logging
		}
	}()
	
	// Try to start handler
	log.Println("Starting AWS Lambda handler...")
	aws.StartLambdaHandler()
	
	// This should never be reached
	log.Println("WARNING: Lambda handler exited")
	fmt.Println("Lambda handler exited unexpectedly")
}