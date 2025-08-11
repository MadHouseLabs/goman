package ui

import (
	"strings"
	
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	zone "github.com/lrstanley/bubblezone"
	ui "github.com/madhouselabs/goman/internal/ui"
	"github.com/madhouselabs/goman/pkg/models"
	"github.com/madhouselabs/goman/pkg/storage"
)

// RenderLoading renders a loading screen
func RenderLoading(width, height int, message string, spinner spinner.Model) string {
	return RenderProLoading(message)
}

// RenderClusterList renders the cluster list view
func RenderClusterList(width, height int, clusters []models.K3sCluster, selectedIndex int, message string, err error, keys KeyMap) string {
	// Determine status based on message/error
	var status ui.StatusType
	var statusMsg string
	
	if err != nil {
		status = ui.StatusError
		statusMsg = err.Error()
	} else if message != "" {
		if strings.Contains(strings.ToLower(message), "setting") || strings.Contains(strings.ToLower(message), "creating") {
			status = ui.StatusSettingUp
			statusMsg = message
		} else {
			status = ui.StatusReady
			statusMsg = message
		}
	} else {
		status = ui.StatusReady
	}
	
	// Get list content
	if len(clusters) == 0 {
		return ui.RenderEmptyViewport(width, height, status, statusMsg)
	}
	
	// Render clusters list with mouse zones
	content := RenderProListWithWidth(clusters, selectedIndex, width)
	
	// Create viewport with zones
	viewport := ui.RenderViewport(width, height, content, status, statusMsg)
	
	// Scan for mouse zones
	return zone.Scan(viewport)
}

// RenderCreateForm renders the cluster creation form
func RenderCreateForm(width, height int, form *ProForm) string {
	if form == nil {
		return "No form available"
	}
	return form.View()
}

// RenderClusterDetail renders the cluster detail view
func RenderClusterDetail(width, height int, cluster *models.K3sCluster, viewport viewport.Model) string {
	if cluster == nil {
		return "No cluster selected"
	}
	return RenderProDetail(*cluster)
}

// RenderDeleteConfirmation renders the delete confirmation dialog
func RenderDeleteConfirmation(clusterName string, width, height int) string {
	content := ui.RenderConfirmationDialog(
		"Delete Cluster",
		"Are you sure you want to delete cluster \""+clusterName+"\"?",
		"This action cannot be undone.",
		"[Y]es / [N]o",
	)
	
	return ui.RenderViewport(width, height, content, ui.StatusWarning, "")
}

// RenderClusterListWithStates renders the cluster list with states in a viewport
func RenderClusterListWithStates(width, height int, clusters []models.K3sCluster, states map[string]*storage.K3sClusterState, selectedIndex int) string {
	// Determine status
	status := ui.StatusReady
	statusMsg := ""
	
	// Get list content
	if len(clusters) == 0 {
		return ui.RenderEmptyViewport(width, height, status, statusMsg)
	}
	
	// Render clusters list with states
	content := RenderProListWithStatesAndWidth(clusters, states, selectedIndex, width)
	
	// Create viewport with proper title and height
	viewport := ui.RenderViewport(width, height, content, status, statusMsg)
	
	// Scan for mouse zones
	return zone.Scan(viewport)
}