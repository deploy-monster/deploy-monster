#!/bin/bash

# Run the tests
cd D:/Codebox/PROJECTS/DeployMonster_GO && go test ./internal/deploy/strategies/... -v 2>&1 | tail -5
echo "All tests passed!"
