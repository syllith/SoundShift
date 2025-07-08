package fyneCustom

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// . ScrollableSlider is a custom slider that can be adjusted using scroll events
type ScrollableSlider struct {
	widget.Slider
}

// . NewScrollableSlider creates a new ScrollableSlider with specified min and max values
func NewScrollableSlider(min, max float64) *ScrollableSlider {
	s := &ScrollableSlider{}
	s.Min = min
	s.Max = max
	s.ExtendBaseWidget(s) // Initialize as a Fyne widget
	return s
}

// . Scrolled adjusts the slider's value based on scroll events
func (s *ScrollableSlider) Scrolled(ev *fyne.ScrollEvent) {
	//* Define the increment amount as 1/20th of the slider's range
	increment := (s.Max - s.Min) / 20 // Adjust increment as needed

	//* Increase or decrease the slider value based on scroll direction
	if ev.Scrolled.DY > 0 {
		s.Value += increment
	} else if ev.Scrolled.DY < 0 {
		s.Value -= increment
	}

	//* Ensure the slider value stays within the defined min and max bounds
	if s.Value > s.Max {
		s.Value = s.Max
	} else if s.Value < s.Min {
		s.Value = s.Min
	}

	s.Refresh() // Update the slider display

	//* Trigger the OnChanged callback if defined
	if s.OnChanged != nil {
		s.OnChanged(s.Value)
	}
}

// . MouseIn is required to satisfy the desktop.Hoverable interface
func (s *ScrollableSlider) MouseIn(*desktop.MouseEvent) {}

// . MouseMoved is required to satisfy the desktop.Hoverable interface
func (s *ScrollableSlider) MouseMoved(*desktop.MouseEvent) {}

// . MouseOut is required to satisfy the desktop.Hoverable interface
func (s *ScrollableSlider) MouseOut() {}

// . ColorButton is a button with customizable background color and icon
type ColorButton struct {
	widget.Button
	backgroundColor color.Color   // Background color of the button
	icon            fyne.Resource // Icon displayed on the button
}

// . colorButtonRenderer is responsible for rendering the ColorButton's appearance
type colorButtonRenderer struct {
	button       *ColorButton      // Reference to the ColorButton being rendered
	textRenderer *canvas.Text      // Renderer for the button text
	iconRenderer *canvas.Image     // Renderer for the button icon
	bgRenderer   *canvas.Rectangle // Renderer for the button's background color
}

// . NewColorButton creates a button with a custom background color, icon, and tap handler
func NewColorButton(label string, bgColor color.Color, icon fyne.Resource, tapped func()) *ColorButton {
	btn := &ColorButton{
		Button:          *widget.NewButtonWithIcon(label, icon, tapped),
		backgroundColor: bgColor,
		icon:            icon,
	}
	btn.ExtendBaseWidget(btn) // Initialize as a Fyne widget
	return btn
}

func (c *ColorButton) CreateRenderer() fyne.WidgetRenderer {
	// . CreateRenderer sets up the renderer for ColorButton with background, text, and icon renderers
	return &colorButtonRenderer{
		button:       c,
		textRenderer: canvas.NewText(c.Text, color.Black), // Renderer for button text, defaulting to black color
		iconRenderer: canvas.NewImageFromResource(c.icon), // Renderer for the button icon
		bgRenderer: &canvas.Rectangle{ // Renderer for the button background
			FillColor:   c.backgroundColor, // Set background fill color
			StrokeColor: c.backgroundColor, // Set background stroke color (same as fill for simplicity)
		},
	}
}

// . Destroy is a no-op required to satisfy the WidgetRenderer interface
func (r *colorButtonRenderer) Destroy() {}

// . Layout arranges the ColorButton's components (background, icon, text) based on the provided size
func (r *colorButtonRenderer) Layout(size fyne.Size) {
	// Padding for icon and text
	const iconPadding = 6
	const textPadding = 8

	text := r.button.Text
	if text == "" {
		// Icon-only button: center icon and background in button
		iconSize := size.Width
		if size.Height < size.Width {
			iconSize = size.Height
		}
		iconSize = iconSize - iconPadding*2
		if iconSize < 0 {
			iconSize = 0
		}
		x := (size.Width - iconSize) / 2
		y := (size.Height - iconSize) / 2
		r.iconRenderer.Resize(fyne.NewSize(iconSize, iconSize))
		r.iconRenderer.Move(fyne.NewPos(x, y))
		// Move and resize background to match icon
		r.bgRenderer.Resize(fyne.NewSize(iconSize, iconSize))
		r.bgRenderer.Move(fyne.NewPos(x, y))
		// Hide text
		r.textRenderer.Resize(fyne.NewSize(0, 0))
		r.textRenderer.Move(fyne.NewPos(0, 0))
	} else {
		// Layout background to full size
		r.bgRenderer.Resize(size)
		// Layout icon: place on the right, vertically centered
		iconSize := size.Height - iconPadding*2
		if iconSize < 0 {
			iconSize = 0
		}
		r.iconRenderer.Resize(fyne.NewSize(iconSize, iconSize))
		r.iconRenderer.Move(fyne.NewPos(size.Width-iconSize-iconPadding, iconPadding))

		// Layout text: fill remaining space, with padding
		textWidth := size.Width - iconSize - iconPadding - textPadding
		if textWidth < 0 {
			textWidth = 0
		}
		r.textRenderer.Resize(fyne.NewSize(textWidth, size.Height-2*textPadding))
		r.textRenderer.Move(fyne.NewPos(textPadding, textPadding))
	}
}

// . MinSize calculates the minimum size required to display the ColorButton's text and icon
func (r *colorButtonRenderer) MinSize() fyne.Size {
	// Calculate minimum size for icon-only or text+icon button
	const iconPadding = 6
	const textPadding = 8
	text := r.button.Text
	iconMin := r.iconRenderer.MinSize()
	if text == "" {
		// Icon-only button: just icon plus padding
		return fyne.NewSize(iconMin.Width+iconPadding*2, iconMin.Height+iconPadding*2)
	}
	// Text + icon button
	textMin := r.textRenderer.MinSize()
	width := textMin.Width + iconMin.Width + iconPadding + textPadding*2
	height := textMin.Height
	if iconMin.Height+iconPadding*2 > height {
		height = iconMin.Height + iconPadding*2
	}
	return fyne.NewSize(width, height)
}

// . Objects returns all drawable components of the ColorButton for rendering
func (r *colorButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bgRenderer, r.textRenderer, r.iconRenderer}
}

// . Refresh updates the ColorButton's visual elements to reflect any state changes
func (r *colorButtonRenderer) Refresh() {
	//* Refresh each renderer to apply any updates (e.g., text, icon, or background color changes)
	r.textRenderer.Refresh()
	r.iconRenderer.Refresh()
	r.bgRenderer.Refresh()
}

// . BackgroundColor returns the button's background color for use in rendering
func (r *colorButtonRenderer) BackgroundColor() color.Color {
	return r.button.backgroundColor
}

// . CustomVBoxLayout is a vertical box layout with a fixed width and centered objects
type CustomVBoxLayout struct {
	FixedWidth float32 // Width to apply to each child object in the layout
}

// . Layout arranges child objects vertically with fixed width and centered alignment
func (c *CustomVBoxLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)            // Initial vertical position
	padding := theme.Padding() // Padding between elements

	//* Calculate the horizontal position to center objects based on the fixed width
	x := (size.Width - c.FixedWidth) / 2

	for _, obj := range objects {
		if obj.Visible() {
			//* Move each object to the calculated horizontal position and current vertical position
			obj.Move(fyne.NewPos(x, y))

			//* Resize each object to the fixed width, while maintaining its minimum height
			obj.Resize(fyne.NewSize(c.FixedWidth, obj.MinSize().Height))

			//* Increment y position by the height of the object and padding for the next element
			y += obj.MinSize().Height + padding
		}
	}
}

// . MinSize calculates the minimum size required to fit all visible objects vertically
func (c *CustomVBoxLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	totalHeight := float32(0)  // Accumulator for total height of all objects
	padding := theme.Padding() // Padding between objects

	//* Sum the height of all visible objects, including padding between them
	for _, obj := range objects {
		if obj.Visible() {
			totalHeight += obj.MinSize().Height + padding
		}
	}

	//* Remove extra padding added after the last element
	if len(objects) > 0 {
		totalHeight -= padding
	}

	//* Return the fixed width and calculated total height as the minimum size
	return fyne.NewSize(c.FixedWidth, totalHeight)
}
