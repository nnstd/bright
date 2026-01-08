package raft

import (
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
)

// DiscoveryConfig contains configuration for Kubernetes peer discovery
type DiscoveryConfig struct {
	K8sServiceName string // e.g., "bright"
	K8sNamespace   string // e.g., "default"
	RaftPort       int    // e.g., 7000
}

// DiscoverPeers uses Kubernetes headless service DNS for peer discovery
// Returns a list of peer addresses in the format "IP:PORT"
func DiscoverPeers(config DiscoveryConfig) ([]string, error) {
	// Construct headless service DNS name
	serviceDNS := fmt.Sprintf("%s.%s.svc.cluster.local", config.K8sServiceName, config.K8sNamespace)

	// Lookup all pod IPs behind the service
	ips, err := net.LookupIP(serviceDNS)
	if err != nil {
		return nil, fmt.Errorf("failed to lookup service %s: %w", serviceDNS, err)
	}

	peers := make([]string, 0, len(ips))
	for _, ip := range ips {
		peers = append(peers, fmt.Sprintf("%s:%d", ip.String(), config.RaftPort))
	}

	return peers, nil
}

// GetNodeIDFromHostname extracts node ID from Kubernetes pod hostname
// Expects StatefulSet format: bright-0, bright-1, etc.
// Returns node ID in format: node-0, node-1, etc.
func GetNodeIDFromHostname() (string, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return "", fmt.Errorf("failed to get hostname: %w", err)
	}

	// Extract ordinal from StatefulSet pod name
	parts := strings.Split(hostname, "-")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid hostname format: %s (expected format: <name>-<ordinal>)", hostname)
	}

	// Get the last part as ordinal
	ordinalStr := parts[len(parts)-1]
	ordinal, err := strconv.Atoi(ordinalStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse ordinal from hostname %s: %w", hostname, err)
	}

	return fmt.Sprintf("node-%d", ordinal), nil
}
