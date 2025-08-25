#!/bin/bash
# Debug script for reconciler

echo "Starting debugger on port 2345..."
echo ""
echo "Set breakpoints at:"
echo "  break pkg/controller/reconciler.go:176"
echo "  break pkg/provider/aws/compute_service.go:384"
echo ""
echo "Then type 'continue' to run"
echo ""

~/go/bin/dlv debug cmd/test-reconciler/main.go --headless --listen=:2345 --api-version=2 -- k3s-cluster-1756046400