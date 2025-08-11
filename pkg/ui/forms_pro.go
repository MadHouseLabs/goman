package ui

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	zone "github.com/lrstanley/bubblezone"
	styles "github.com/madhouselabs/goman/internal/ui"
	"github.com/madhouselabs/goman/pkg/config"
	"github.com/madhouselabs/goman/pkg/models"
)

// ProForm represents a professional minimal form
type ProForm struct {
	title      string
	fields     []ProFormField
	focusIndex int
	errors     map[int]string
	submitted  bool
	Submitted  bool // Public field for checking submission status
	isUpdate   bool // Whether this is an update form
	dropdownOpen int // -1 if no dropdown is open, otherwise the field index
	dropdownIndex int // Selected index in the dropdown
}

// ProFormField represents a minimal form field
type ProFormField struct {
	Label      string
	Value      string
	Placeholder string
	Required   bool
	Validator  func(string) error
	Input      textinput.Model
	IsDropdown bool
	Options    []string
	DropdownOpen bool
	SelectedIndex int
	SearchTerm string
	FilteredOptions []string
	FilteredIndices []int // Maps filtered options to original indices
}

// NewProClusterForm creates a professional cluster creation form with smart defaults
func NewProClusterForm() *ProForm {
	return NewProClusterFormWithConfig(nil)
}

// NewProUpdateForm creates a professional cluster update form with existing cluster data
func NewProUpdateForm(cluster *models.K3sCluster) *ProForm {
	form := &ProForm{
		title:    "Update Cluster",
		fields:   []ProFormField{},
		errors:   make(map[int]string),
		isUpdate: true,
	}
	
	// Pre-populate fields with existing cluster data
	nodeCount := len(cluster.MasterNodes) + len(cluster.WorkerNodes)
	
	// Extract version without +k3s1 suffix
	version := strings.TrimSuffix(cluster.K3sVersion, "+k3s1")
	
	// Get instance type from first node
	instanceType := "t3.medium"
	if len(cluster.MasterNodes) > 0 {
		instanceType = cluster.MasterNodes[0].InstanceType
	}
	
	// Get region from first node
	region := "ap-south-1"
	if len(cluster.MasterNodes) > 0 {
		region = cluster.MasterNodes[0].Region
	}
	
	// Join tags
	tags := strings.Join(cluster.Tags, ", ")
	
	// Create fields with existing values
	fields := []struct {
		label       string
		placeholder string
		defaultVal  string
		required    bool
		editable    bool
		validator   func(string) error
	}{
		{
			label:       "Name",
			placeholder: cluster.Name,
			defaultVal:  cluster.Name,
			required:    true,
			editable:    false, // Name cannot be changed
			validator:   nil,
		},
		{
			label:       "Region",
			placeholder: region,
			defaultVal:  region,
			required:    true,
			editable:    false, // Region cannot be changed
			validator:   nil,
		},
		{
			label:       "Version",
			placeholder: version,
			defaultVal:  version,
			required:    true,
			editable:    true,
			validator: func(s string) error {
				validVersions := []string{"1.28", "1.27", "1.26", "1.29"}
				for _, v := range validVersions {
					if strings.HasPrefix(s, v) {
						return nil
					}
				}
				return fmt.Errorf("use: 1.28, 1.27, or 1.26")
			},
		},
		{
			label:       "Nodes",
			placeholder: fmt.Sprintf("%d", nodeCount),
			defaultVal:  fmt.Sprintf("%d", nodeCount),
			required:    true,
			editable:    true,
			validator: func(s string) error {
				n, err := strconv.Atoi(s)
				if err != nil {
					return fmt.Errorf("number required")
				}
				if n < 1 || n > 100 {
					return fmt.Errorf("1-100 range")
				}
				return nil
			},
		},
		{
			label:       "Instance",
			placeholder: instanceType,
			defaultVal:  instanceType,
			required:    true,
			editable:    true,
			validator: func(s string) error {
				validTypes := []string{"t3.micro", "t3.small", "t3.medium", "t3.large", "t3.xlarge"}
				for _, t := range validTypes {
					if s == t {
						return nil
					}
				}
				return fmt.Errorf("use: t3.small, t3.medium, t3.large")
			},
		},
		{
			label:       "Tags",
			placeholder: tags,
			defaultVal:  tags,
			required:    false,
			editable:    true,
			validator:   nil,
		},
	}
	
	for _, f := range fields {
		input := textinput.New()
		input.Placeholder = f.placeholder
		input.CharLimit = 50
		input.Width = 28
		
		// Pre-fill with existing value
		input.SetValue(f.defaultVal)
		
		// Disable non-editable fields
		if !f.editable {
			input.Blur()
		}
		
		// Configure input styling to match our theme
		input.PromptStyle = lipgloss.NewStyle().Foreground(styles.ColorWhite)
		input.TextStyle = lipgloss.NewStyle().Foreground(styles.ColorWhite)
		input.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.ColorGray)
		
		form.fields = append(form.fields, ProFormField{
			Label:       f.label,
			Placeholder: f.placeholder,
			Required:    f.required,
			Validator:   f.validator,
			Input:       input,
		})
	}
	
	// Focus first editable field
	for i, field := range fields {
		if field.editable {
			form.fields[i].Input.Focus()
			form.focusIndex = i
			break
		}
	}
	
	return form
}

// NewProClusterFormWithConfig creates a form with config-based defaults
func NewProClusterFormWithConfig(cfg *config.Config) *ProForm {
	form := &ProForm{
		title:  "Create Cluster",
		fields: []ProFormField{},
		errors: make(map[int]string),
		dropdownOpen: -1,
		dropdownIndex: 0,
	}

	// Get smart defaults
	region := "ap-south-1"
	instanceType := "t3.medium"
	version := "v1.28.5+k3s1"
	tags := "dev"
	
	if cfg != nil {
		region = cfg.AWSRegion
		instanceType = cfg.InstanceType
		version = cfg.K3sVersion
	}

	// Generate a suggested cluster name based on context
	suggestedName := generateClusterName()
	
	// Available options for dropdowns
	regionOptions := []string{"us-west-1", "us-west-2", "us-east-1", "us-east-2", "eu-west-1", "eu-central-1", "ap-south-1"}
	instanceOptions := []string{"t3.micro", "t3.small", "t3.medium", "t3.large", "t3.xlarge"}

	// Create fields with smart defaults
	fields := []struct {
		label       string
		placeholder string
		defaultVal  string
		required    bool
		isDropdown  bool
		options     []string
		validator   func(string) error
	}{
		{
			label:       "Name",
			placeholder: suggestedName,
			defaultVal:  "",  // Don't pre-fill name, just suggest
			required:    true,
			isDropdown:  false,
			options:     nil,
			validator: func(s string) error {
				if s == "" {
					return fmt.Errorf("required")
				}
				if len(s) < 3 {
					return fmt.Errorf("min 3 chars")
				}
				return nil
			},
		},
		{
			label:       "Region",
			placeholder: region,
			defaultVal:  region,  // Pre-fill with last used
			required:    true,
			isDropdown:  true,
			options:     regionOptions,
			validator: func(s string) error {
				for _, r := range regionOptions {
					if s == r {
						return nil
					}
				}
				return fmt.Errorf("invalid region")
			},
		},
		{
			label:       "Version",
			placeholder: version,
			defaultVal:  version,  // Pre-fill with last used
			required:    true,
			isDropdown:  true,
			options:     []string{"v1.29.5+k3s1", "v1.28.5+k3s1", "v1.27.10+k3s1", "v1.26.13+k3s1"},
			validator: func(s string) error {
				validVersions := []string{"v1.29", "v1.28", "v1.27", "v1.26"}
				for _, v := range validVersions {
					if strings.HasPrefix(s, v) {
						return nil
					}
				}
				return fmt.Errorf("invalid version")
			},
		},
		{
			label:       "Nodes",
			placeholder: "3 (HA mode)",
			defaultVal:  "3 (HA mode)",  // Default to HA mode
			required:    true,
			isDropdown:  true,
			options:     []string{"1 (Dev mode)", "3 (HA mode)"},
			validator: func(s string) error {
				// Extract just the number from options like "1 (Dev mode)"
				parts := strings.Fields(s)
				if len(parts) > 0 {
					n, err := strconv.Atoi(parts[0])
					if err == nil && (n == 1 || n == 3) {
						return nil
					}
				}
				return fmt.Errorf("invalid node count")
			},
		},
		{
			label:       "Instance",
			placeholder: instanceType,
			defaultVal:  instanceType,  // Pre-fill with last used
			required:    true,
			isDropdown:  true,
			options:     instanceOptions,
			validator: func(s string) error {
				for _, t := range instanceOptions {
					if s == t {
						return nil
					}
				}
				return fmt.Errorf("invalid instance type")
			},
		},
		{
			label:       "Tags",
			placeholder: tags,
			defaultVal:  tags,  // Pre-fill with last used
			required:    false,
			isDropdown:  false,
			options:     nil,
			validator:   nil,
		},
	}

	for _, f := range fields {
		input := textinput.New()
		input.Placeholder = f.placeholder
		input.CharLimit = 50
		input.Width = 28  // Match the visual width of our styled input
		
		// Pre-fill with default value if provided
		if f.defaultVal != "" {
			input.SetValue(f.defaultVal)
		}
		
		// Configure input styling to match our theme
		input.PromptStyle = lipgloss.NewStyle().Foreground(styles.ColorWhite)
		input.TextStyle = lipgloss.NewStyle().Foreground(styles.ColorWhite)
		input.PlaceholderStyle = lipgloss.NewStyle().Foreground(styles.ColorGray)
		
		// Find selected index for dropdown
		selectedIdx := 0
		if f.isDropdown && f.defaultVal != "" {
			for i, opt := range f.options {
				if opt == f.defaultVal {
					selectedIdx = i
					break
				}
			}
		}
		
		field := ProFormField{
			Label:       f.label,
			Placeholder: f.placeholder,
			Required:    f.required,
			Validator:   f.validator,
			Input:       input,
			IsDropdown:  f.isDropdown,
			Options:     f.options,
			DropdownOpen: false,
			SelectedIndex: selectedIdx,
			SearchTerm: "",
		}
		
		// Initialize filtered options for dropdowns
		if f.isDropdown {
			// Initialize with all options (filter will limit to 3 when needed)
			field.FilteredOptions = f.options
			field.FilteredIndices = make([]int, len(f.options))
			for i := range f.options {
				field.FilteredIndices[i] = i
			}
			// Limit display to 3 if more than 3 options
			if len(field.FilteredOptions) > 3 {
				field.FilteredOptions = field.FilteredOptions[:3]
				field.FilteredIndices = field.FilteredIndices[:3]
			}
		}
		
		form.fields = append(form.fields, field)
	}

	// Focus first field
	if len(form.fields) > 0 {
		form.fields[0].Input.Focus()
	}

	return form
}

// Update handles form input
func (f *ProForm) Update(msg tea.Msg) tea.Cmd {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.MouseMsg:
		// Handle mouse clicks on zones
		if msg.Type == tea.MouseRelease && msg.Button == tea.MouseButtonLeft {
			// Check field clicks
			for i := range f.fields {
				zoneID := fmt.Sprintf("field_%d", i)
				if zone.Get(zoneID).InBounds(msg) {
					f.focusIndex = i
					f.updateFocus()
					return nil
				}
			}
			
			// Check submit button click
			if zone.Get("button_submit").InBounds(msg) {
				f.focusIndex = len(f.fields)
				if f.Validate() {
					f.submitted = true
					f.Submitted = true
					return nil
				}
			}
			
			// Check cancel button click
			if zone.Get("button_cancel").InBounds(msg) {
				return tea.Quit
			}
		}
	
	case tea.KeyMsg:
		// Check if a dropdown is open
		if f.dropdownOpen >= 0 && f.focusIndex < len(f.fields) {
			field := &f.fields[f.focusIndex]
			if field.IsDropdown {
				switch msg.String() {
				case "up":
					if field.SelectedIndex > 0 {
						field.SelectedIndex--
					}
					return nil
				case "down":
					if len(field.FilteredOptions) > 0 && field.SelectedIndex < len(field.FilteredOptions)-1 {
						field.SelectedIndex++
					}
					return nil
				case "enter":
					// Select the option from filtered list
					if len(field.FilteredOptions) > 0 && field.SelectedIndex < len(field.FilteredOptions) {
						originalIdx := field.FilteredIndices[field.SelectedIndex]
						field.Input.SetValue(field.Options[originalIdx])
						field.Value = field.Options[originalIdx]
					}
					field.DropdownOpen = false
					f.dropdownOpen = -1
					field.SearchTerm = ""
					return nil
				case "esc":
					// Close dropdown without selecting
					field.DropdownOpen = false
					f.dropdownOpen = -1
					field.SearchTerm = ""
					return nil
				case "backspace":
					// Remove last character from search
					if len(field.SearchTerm) > 0 {
						field.SearchTerm = field.SearchTerm[:len(field.SearchTerm)-1]
						f.filterDropdownOptions(field)
					}
					return nil
				default:
					// Check if it's a character to add to search
					if len(msg.String()) == 1 {
						field.SearchTerm += msg.String()
						f.filterDropdownOptions(field)
						// Reset selection to first item
						field.SelectedIndex = 0
					}
					return nil
				}
			}
		}
		
		// Normal navigation
		switch msg.String() {
		case "tab":
			f.NextField()
		case "shift+tab":
			f.PrevField()
		case "down":
			// Just move to next field
			f.NextField()
		case "up":
			f.PrevField()
		case "esc":
			// ESC key cancels the form
			return tea.Quit
		case "enter":
			if f.focusIndex == len(f.fields) {
				// Submit button
				if f.Validate() {
					f.submitted = true
					f.Submitted = true // Set public field
					return nil // Form is valid
				}
				return nil
			} else if f.focusIndex == len(f.fields) + 1 {
				// Cancel button
				return tea.Quit
			} else if f.focusIndex < len(f.fields) {
				// Check if it's a dropdown field
				field := &f.fields[f.focusIndex]
				if field.IsDropdown {
					if !field.DropdownOpen {
						// Open dropdown
						field.DropdownOpen = true
						f.dropdownOpen = f.focusIndex
						field.SearchTerm = ""
						f.filterDropdownOptions(field)
						// Find current value in filtered options
						field.SelectedIndex = 0
						for i, opt := range field.FilteredOptions {
							if opt == field.Input.Value() {
								field.SelectedIndex = i
								break
							}
						}
						return nil
					} else {
						// Select current option from filtered list
						if len(field.FilteredOptions) > 0 && field.SelectedIndex < len(field.FilteredOptions) {
							originalIdx := field.FilteredIndices[field.SelectedIndex]
							field.Input.SetValue(field.Options[originalIdx])
							field.Value = field.Options[originalIdx]
						}
						// Close dropdown
						field.DropdownOpen = false
						f.dropdownOpen = -1
						field.SearchTerm = ""
						return nil
					}
				} else {
					// Move to next field for non-dropdown
					f.NextField()
				}
			}
		case " ":
			// Space key handling for dropdowns
			if f.focusIndex < len(f.fields) {
				field := &f.fields[f.focusIndex]
				if field.IsDropdown && !field.DropdownOpen {
					field.DropdownOpen = true
					f.dropdownOpen = f.focusIndex
					field.SearchTerm = ""
					f.filterDropdownOptions(field)
					// Find current value in options
					field.SelectedIndex = 0
					for i, opt := range field.FilteredOptions {
						if opt == field.Input.Value() {
							field.SelectedIndex = i
							break
						}
					}
					return nil
				}
			}
		}
	}

	// Update current field (only if it's not a dropdown)
	if f.focusIndex < len(f.fields) {
		field := &f.fields[f.focusIndex]
		// Don't allow direct editing of dropdown fields
		if !field.IsDropdown {
			var cmd tea.Cmd
			field.Input, cmd = field.Input.Update(msg)
			cmds = append(cmds, cmd)
		}
	}

	return tea.Batch(cmds...)
}

// NextField moves to next field
func (f *ProForm) NextField() {
	f.focusIndex++
	if f.focusIndex > len(f.fields) + 1 { // +1 for Submit, +1 for Cancel
		f.focusIndex = 0
	}
	f.updateFocus()
}

// PrevField moves to previous field
func (f *ProForm) PrevField() {
	f.focusIndex--
	if f.focusIndex < 0 {
		f.focusIndex = len(f.fields) + 1 // Go to Cancel button
	}
	f.updateFocus()
}

// updateFocus updates which field has focus
func (f *ProForm) updateFocus() {
	for i := range f.fields {
		if i == f.focusIndex {
			f.fields[i].Input.Focus()
		} else {
			f.fields[i].Input.Blur()
		}
	}
}

// filterDropdownOptions filters dropdown options based on search term
func (f *ProForm) filterDropdownOptions(field *ProFormField) {
	field.FilteredOptions = []string{}
	field.FilteredIndices = []int{}
	
	searchLower := strings.ToLower(field.SearchTerm)
	
	// Filter options that contain the search term
	for i, option := range field.Options {
		if searchLower == "" || strings.Contains(strings.ToLower(option), searchLower) {
			field.FilteredOptions = append(field.FilteredOptions, option)
			field.FilteredIndices = append(field.FilteredIndices, i)
			
			// Limit to max 3 options
			if len(field.FilteredOptions) >= 3 {
				break
			}
		}
	}
	
	// Reset selection if out of bounds
	if field.SelectedIndex >= len(field.FilteredOptions) {
		field.SelectedIndex = 0
	}
}

// Validate validates all fields
func (f *ProForm) Validate() bool {
	f.errors = make(map[int]string)
	isValid := true

	for i, field := range f.fields {
		value := field.Input.Value()
		
		if field.Required && value == "" {
			f.errors[i] = "required"
			isValid = false
			continue
		}
		
		if field.Validator != nil && value != "" {
			if err := field.Validator(value); err != nil {
				f.errors[i] = err.Error()
				isValid = false
			}
		}
	}

	return isValid
}


// IsSubmitted returns true if the form has been submitted
func (f *ProForm) IsSubmitted() bool {
	return f.submitted
}

// GetCluster returns cluster from form data
func (f *ProForm) GetCluster() models.K3sCluster {
	// Get values - use Value field for dropdowns
	nameValue := f.fields[0].Input.Value()
	regionValue := f.fields[1].Input.Value()
	if f.fields[1].IsDropdown && f.fields[1].Value != "" {
		regionValue = f.fields[1].Value
	}
	versionValue := f.fields[2].Input.Value()
	if f.fields[2].IsDropdown && f.fields[2].Value != "" {
		versionValue = f.fields[2].Value
	}
	nodeCountStr := f.fields[3].Input.Value()
	if f.fields[3].IsDropdown && f.fields[3].Value != "" {
		nodeCountStr = f.fields[3].Value
	}
	instanceValue := f.fields[4].Input.Value()
	if f.fields[4].IsDropdown && f.fields[4].Value != "" {
		instanceValue = f.fields[4].Value
	}
	tagsValue := f.fields[5].Input.Value()
	
	// Extract node count from string like "3 (HA mode)"
	totalNodeCount := 3 // default
	if strings.HasPrefix(nodeCountStr, "1") {
		totalNodeCount = 1
	} else if strings.HasPrefix(nodeCountStr, "3") {
		totalNodeCount = 3
	}
	
	tags := []string{}
	if tagsValue != "" {
		for _, tag := range strings.Split(tagsValue, ",") {
			tags = append(tags, strings.TrimSpace(tag))
		}
	}

	// Create nodes
	var masterNodes []models.Node
	var workerNodes []models.Node

	masterCount := 1
	workerCount := 0
	
	if totalNodeCount == 3 {
		// HA mode: 3 masters, no workers
		masterCount = 3
		workerCount = 0
	} else if totalNodeCount == 1 {
		// Dev mode: 1 master, no workers
		masterCount = 1
		workerCount = 0
	}

	for i := 0; i < masterCount; i++ {
		masterNodes = append(masterNodes, models.Node{
			ID:           fmt.Sprintf("node-m%d", i+1),
			Name:         fmt.Sprintf("master-%d", i+1),
			Role:         models.RoleMaster,
			IP:           fmt.Sprintf("10.0.1.%d", 10+i),
			Status:       "Ready",
			CPU:          4,
			MemoryGB:     8,
			StorageGB:    100,
			Provider:     "AWS",
			InstanceType: instanceValue,
			Region:       regionValue,
		})
	}

	for i := 0; i < workerCount; i++ {
		workerNodes = append(workerNodes, models.Node{
			ID:           fmt.Sprintf("node-w%d", i+1),
			Name:         fmt.Sprintf("worker-%d", i+1),
			Role:         models.RoleWorker,
			IP:           fmt.Sprintf("10.0.2.%d", 10+i),
			Status:       "Ready",
			CPU:          4,
			MemoryGB:     8,
			StorageGB:    100,
			Provider:     "AWS",
			InstanceType: instanceValue,
			Region:       regionValue,
		})
	}

	// Clean up version - remove +k3s1 for KubeVersion
	kubeVersion := versionValue
	if strings.Contains(kubeVersion, "+") {
		kubeVersion = strings.Split(kubeVersion, "+")[0]
	}
	
	// Determine mode based on node count
	mode := models.ModeHA
	if totalNodeCount == 1 {
		mode = models.ModeDeveloper
	}
	
	return models.K3sCluster{
		Name:           nameValue,
		Region:         regionValue,
		Mode:           mode,
		InstanceType:   instanceValue,
		K3sVersion:     versionValue,
		KubeVersion:    kubeVersion,
		MasterNodes:    masterNodes,
		WorkerNodes:    workerNodes,
		Tags:           tags,
		NetworkCIDR:    "10.42.0.0/16",
		ServiceCIDR:    "10.43.0.0/16",
		ClusterDNS:     "10.43.0.10",
		SSHKeyPath:     "~/.ssh/k3s_rsa",
		Features: models.K3sFeatures{
			Traefik:        true,
			ServiceLB:      true,
			LocalStorage:   true,
			MetricsServer:  true,
			CoreDNS:        true,
			FlannelBackend: "vxlan",
		},
	}
}

// generateClusterName generates a smart cluster name suggestion
func generateClusterName() string {
	// Use environment-based prefixes
	hour := time.Now().Hour()
	var prefix string
	
	if hour >= 6 && hour < 12 {
		prefix = "dev"
	} else if hour >= 12 && hour < 18 {
		prefix = "test"
	} else if hour >= 18 && hour < 22 {
		prefix = "staging"
	} else {
		prefix = "prod"
	}
	
	// Add a random suffix for uniqueness
	adjectives := []string{"swift", "nimble", "rapid", "agile", "smart", "bright", "nova", "apex"}
	rand.Seed(time.Now().UnixNano())
	adj := adjectives[rand.Intn(len(adjectives))]
	
	return fmt.Sprintf("%s-%s-k3s", prefix, adj)
}

// View renders the form
func (f *ProForm) View() string {
	// Use the viewport form rendering
	return f.RenderViewport(80, 30)
}

// RenderViewport renders the form in viewport style
func (f *ProForm) RenderViewport(width, height int) string {
	var fields []styles.FormField
	
	// Find which dropdown is open
	openDropdownIndex := -1
	
	for i := range f.fields {
		field := &f.fields[i] // Use pointer to get actual field state
		// For dropdowns, use the Value field if set, otherwise Input.Value()
		value := field.Input.Value()
		if field.IsDropdown && field.Value != "" {
			value = field.Value
		}
		focused := i == f.focusIndex
		
		// Get error message if any
		errMsg := ""
		if err, ok := f.errors[i]; ok {
			errMsg = err
		}
		
		// Determine field type
		var fieldType styles.FieldType
		if field.IsDropdown {
			fieldType = styles.FieldTypeDropdown
			if field.DropdownOpen {
				openDropdownIndex = i
			}
		} else {
			fieldType = styles.FieldTypeText
		}
		
		// Use filtered options if dropdown is open, otherwise all options
		options := field.Options
		selectedIdx := field.SelectedIndex
		if field.DropdownOpen && field.FilteredOptions != nil {
			options = field.FilteredOptions
			selectedIdx = field.SelectedIndex // This is already the index in filtered list
		}
		
		fields = append(fields, styles.FormFieldWithDropdownSearch(
			field.Label,
			value,
			field.Placeholder,
			field.Required,
			focused,
			errMsg,
			fieldType,
			options,
			field.DropdownOpen,
			selectedIdx,
			field.SearchTerm,
			field.FilteredOptions,
		))
	}
	
	// Determine status
	var status styles.StatusType
	var statusMsg string
	
	if f.submitted {
		status = styles.StatusSettingUp
		statusMsg = "Creating cluster..."
	} else {
		status = styles.StatusReady
		statusMsg = ""
	}
	
	// Render the form with dropdown index
	output := styles.RenderClusterFormWithDropdown(width, height, f.isUpdate, fields, f.focusIndex, openDropdownIndex, status, statusMsg)
	// Scan the output for mouse zones
	return zone.Scan(output)
}