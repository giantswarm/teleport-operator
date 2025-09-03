/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"testing"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
)

func TestDetectConfigChanges(t *testing.T) {
	testCases := []struct {
		name                  string
		oldConfig             *config.Config
		newConfig             *config.Config
		expectedChanges       int
		expectedHighestImpact ChangeImpact
	}{
		{
			name: "No changes",
			oldConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			newConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			expectedChanges:       0,
			expectedHighestImpact: ImpactLow,
		},
		{
			name: "Critical change - ProxyAddr",
			oldConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			newConfig: &config.Config{
				ProxyAddr:             "new-proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			expectedChanges:       1,
			expectedHighestImpact: ImpactCritical,
		},
		{
			name: "High impact change - ManagementClusterName",
			oldConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			newConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "new-management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			expectedChanges:       1,
			expectedHighestImpact: ImpactHigh,
		},
		{
			name: "Medium impact change - TeleportVersion",
			oldConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			newConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "15.2.0",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			expectedChanges:       1,
			expectedHighestImpact: ImpactMedium,
		},
		{
			name: "Low impact change - AppVersion",
			oldConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			newConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.1",
				AppCatalog:            "giantswarm",
			},
			expectedChanges:       1,
			expectedHighestImpact: ImpactLow,
		},
		{
			name: "Multiple changes - mixed impact",
			oldConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			newConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "15.2.0",         // Medium impact
				ManagementClusterName: "new-management", // High impact
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.1", // Low impact
				AppCatalog:            "giantswarm",
			},
			expectedChanges:       3,
			expectedHighestImpact: ImpactHigh, // Highest of the three changes
		},
		{
			name:      "First config load (nil old config)",
			oldConfig: nil,
			newConfig: &config.Config{
				ProxyAddr:             "proxy.teleport.example.com:443",
				TeleportVersion:       "17.7.5",
				ManagementClusterName: "management",
				AppName:               "teleport-kube-agent",
				AppVersion:            "0.12.0",
				AppCatalog:            "giantswarm",
			},
			expectedChanges:       0, // No changes on first load
			expectedHighestImpact: ImpactLow,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reconciler := &ConfigReconciler{}

			changes := reconciler.detectConfigChanges(tc.oldConfig, tc.newConfig)

			if len(changes) != tc.expectedChanges {
				t.Errorf("Expected %d changes, got %d", tc.expectedChanges, len(changes))
				for _, change := range changes {
					t.Logf("Change: %s %s -> %s (impact: %s)",
						change.Field, change.OldValue, change.NewValue, reconciler.impactString(change.Impact))
				}
			}

			// Check highest impact
			if len(changes) > 0 {
				maxImpact := ImpactLow
				for _, change := range changes {
					if change.Impact > maxImpact {
						maxImpact = change.Impact
					}
				}

				if maxImpact != tc.expectedHighestImpact {
					t.Errorf("Expected highest impact %s, got %s",
						reconciler.impactString(tc.expectedHighestImpact),
						reconciler.impactString(maxImpact))
				}
			}
		})
	}
}

func TestImpactString(t *testing.T) {
	reconciler := &ConfigReconciler{}

	testCases := []struct {
		impact   ChangeImpact
		expected string
	}{
		{ImpactLow, "low"},
		{ImpactMedium, "medium"},
		{ImpactHigh, "high"},
		{ImpactCritical, "critical"},
		{ChangeImpact(999), "unknown"},
	}

	for _, tc := range testCases {
		result := reconciler.impactString(tc.impact)
		if result != tc.expected {
			t.Errorf("Expected impact string %s, got %s", tc.expected, result)
		}
	}
}
