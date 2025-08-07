package cmd

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/aquasecurity/table"
	"github.com/charmbracelet/lipgloss"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/go-connections/nat"
	"github.com/spf13/cobra"
)

// containerInfo holds the processed container information for display
type containerInfo struct {
	name          string
	status        string
	coloredStatus string
	traefikPorts  []string
	externalPorts []string
}

// psCmd represents the ps command (previously "ports" command)
var psCmd = &cobra.Command{
	Use:   "ps",
	Short: "List Docker containers with port mappings",
	Long: `List all Docker containers and their status, displaying their internal
ports (as potentially exposed by Traefik labels) and their external port bindings.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
		if err != nil {
			return err
		}
		defer cli.Close()

		containersSummary, err := cli.ContainerList(ctx, container.ListOptions{All: true})
		if err != nil {
			return err
		}

		// Process container information
		var containers []containerInfo
		for _, cs := range containersSummary {
			containerInspect, err := cli.ContainerInspect(ctx, cs.ID)
			if err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Error inspecting container %s: %v\n", cs.ID[:12], err)
				continue
			}

			internalPorts := getTraefikInternalPorts(containerInspect.Config.Labels)
			deduplicatedInternalPorts := deduplicate(internalPorts)
			externalPorts := getExternalPortBindings(containerInspect.NetworkSettings.Ports)

			containerName := cs.Names[0][1:] // Remove the leading slash

			var statusText string
			var statusStyle lipgloss.Style

			switch containerInspect.State.Status {
			case "running":
				if containerInspect.State.Health != nil && containerInspect.State.Health.Status != "healthy" {
					statusText = fmt.Sprintf("%s (%s)", containerInspect.State.Status, containerInspect.State.Health.Status)
					statusStyle = yellowStyle
				} else {
					statusText = containerInspect.State.Status
					statusStyle = greenStyle
				}
			case "exited":
				statusText = containerInspect.State.Status
				statusStyle = redStyle
			default:
				statusText = containerInspect.State.Status
				statusStyle = yellowStyle // Consider other states as unhealthy/restarting
			}

			coloredStatus := statusStyle.Render(statusText)

			if len(deduplicatedInternalPorts) > 0 || len(externalPorts) > 0 {
				containers = append(containers, containerInfo{
					name:          containerName,
					status:        statusText,
					coloredStatus: coloredStatus,
					traefikPorts:  deduplicatedInternalPorts,
					externalPorts: externalPorts,
				})
			} else {
				containers = append(containers, containerInfo{
					name:          containerName,
					status:        statusText,
					coloredStatus: coloredStatus,
					traefikPorts:  []string{},
					externalPorts: []string{},
				})
			}
		}

		// Sort containers by name
		sort.Slice(containers, func(i, j int) bool {
			return containers[i].name < containers[j].name
		})

		// Create a new table
		t := table.New(cmd.OutOrStdout())

		// Configure table settings
		t.SetHeaders("Container", "Status", "Traefik Port", "Port Bindings")
		t.SetHeaderStyle(table.StyleBold)
		t.SetBorders(true)
		t.SetRowLines(true)
		t.SetDividers(table.UnicodeRoundedDividers)
		t.SetLineStyle(table.StyleBlue)
		t.SetPadding(1)
		t.SetColumnMaxWidth(100)

		// Add sorted containers to the table
		for _, container := range containers {
			traefikPortsStr := strings.Join(container.traefikPorts, ", ")

			// Format external ports to have each on its own line
			externalPortsStr := ""
			if len(container.externalPorts) > 0 {
				externalPortsStr = strings.Join(container.externalPorts, "\n")
			}

			t.AddRow(container.name, container.coloredStatus, traefikPortsStr, externalPortsStr)
		}

		// Render the table
		t.Render()
		return nil
	},
}

func getTraefikInternalPorts(labels map[string]string) []string {
	var internalPorts []string
	for key, value := range labels {
		if strings.Contains(key, "traefik.http.services") &&
			strings.Contains(key, ".loadbalancer.server.port") &&
			!strings.Contains(key, "-http") {
			parts := strings.Split(key, ".")
			if len(parts) > 3 {
				internalPorts = append(internalPorts, fmt.Sprintf("%s", value))
			}
		}
	}
	return internalPorts
}

func getExternalPortBindings(ports nat.PortMap) []string {
	var bindings []string
	for internalPort, hostBindings := range ports {
		for _, binding := range hostBindings {
			if binding.HostPort != "" {
				// Always include the bind IP, even if it's 0.0.0.0
				hostIP := binding.HostIP
				if hostIP == "" {
					hostIP = "0.0.0.0"
				}
				bindingStr := fmt.Sprintf("%s:%s->%s/%s", hostIP, binding.HostPort, internalPort.Port(), internalPort.Proto())
				bindings = append(bindings, bindingStr)
			}
		}
	}
	return bindings
}

func deduplicate(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}
