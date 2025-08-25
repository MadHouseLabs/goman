// +build !aws

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	"github.com/aws/aws-lambda-go/events"
	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/controller"
	"github.com/madhouselabs/goman/pkg/provider/aws"
)

func main() {
	// Set up logging
	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	// Read test event
	eventFile := "test-event.json"
	if len(os.Args) > 1 {
		eventFile = os.Args[1]
	}

	eventData, err := ioutil.ReadFile(eventFile)
	if err != nil {
		log.Fatalf("Failed to read event file: %v", err)
	}

	var s3Event events.S3Event
	if err := json.Unmarshal(eventData, &s3Event); err != nil {
		log.Fatalf("Failed to parse event: %v", err)
	}

	// Set up AWS provider
	profile := config.GetAWSProfile()
	region := config.GetAWSRegion()
	provider, err := aws.GetCachedProvider(profile, region)
	if err != nil {
		log.Fatalf("Failed to get AWS provider: %v", err)
	}

	// Create reconciler
	reconciler, err := controller.NewReconciler(provider, "local-test")
	if err != nil {
		log.Fatalf("Failed to create reconciler: %v", err)
	}

	// Process the event
	ctx := context.Background()
	
	for _, record := range s3Event.Records {
		bucketName := record.S3.Bucket.Name
		objectKey := record.S3.Object.Key
		
		log.Printf("Processing S3 event: bucket=%s, key=%s", bucketName, objectKey)
		
		// Extract cluster name from object key
		// Expected format: clusters/{cluster-name}/config.yaml or clusters/{cluster-name}/status.yaml
		if len(objectKey) > len("clusters/") && (objectKey[len(objectKey)-11:] == "config.yaml" || objectKey[len(objectKey)-11:] == "status.yaml") {
			clusterName := objectKey[len("clusters/"):]
			clusterName = clusterName[:len(clusterName)-12] // Remove /config.yaml or /status.yaml
			
			log.Printf("Triggering reconciliation for cluster: %s", clusterName)
			
			// Run reconciliation
			result, err := reconciler.ReconcileCluster(ctx, clusterName)
			if err != nil {
				log.Printf("Reconciliation failed: %v", err)
			} else {
				log.Printf("Reconciliation result: Requeue=%v, RequeueAfter=%v", 
					result.Requeue, result.RequeueAfter)
			}
		}
	}
	
	log.Println("Local test completed")
}