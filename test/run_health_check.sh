#!/bin/bash

# JiraMate Health Check Script
# This script runs the health check for JiraMate's Jira notification features

# Change to the JiraMate directory
cd /opt/JiraMate/test

# Run the health check
echo "Starting JiraMate health check at $(date)"
go run ./health_check.go

# Exit with the same status code as the Go program
exit $?
