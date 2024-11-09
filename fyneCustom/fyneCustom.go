package fyneCustom

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// * Custom Scrollable Slider
type ScrollableSlider struct {
	widget.Slider
}

func NewScrollableSlider(min, max float64) *ScrollableSlider {
	s := &ScrollableSlider{}
	s.Min = min
	s.Max = max
	s.ExtendBaseWidget(s)
	return s
}

func (s *ScrollableSlider) Scrolled(ev *fyne.ScrollEvent) {
	increment := (s.Max - s.Min) / 20 // Adjust increment as needed
	if ev.Scrolled.DY > 0 {
		s.Value += increment
	} else if ev.Scrolled.DY < 0 {
		s.Value -= increment
	}
	if s.Value > s.Max {
		s.Value = s.Max
	} else if s.Value < s.Min {
		s.Value = s.Min
	}
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(s.Value)
	}
}

// Required to satisfy the desktop.Hoverable interface
func (s *ScrollableSlider) MouseIn(*desktop.MouseEvent)    {}
func (s *ScrollableSlider) MouseMoved(*desktop.MouseEvent) {}
func (s *ScrollableSlider) MouseOut()                      {}

// * ColorButton
type ColorButton struct {
	widget.Button
	backgroundColor color.Color
	icon            fyne.Resource
}

type colorButtonRenderer struct {
	button       *ColorButton
	textRenderer *canvas.Text
	iconRenderer *canvas.Image
	bgRenderer   *canvas.Rectangle
}

func NewColorButton(label string, bgColor color.Color, icon fyne.Resource, tapped func()) *ColorButton {
	btn := &ColorButton{
		Button:          *widget.NewButtonWithIcon(label, icon, tapped),
		backgroundColor: bgColor,
		icon:            icon,
	}
	btn.ExtendBaseWidget(btn)
	return btn
}

func (c *ColorButton) CreateRenderer() fyne.WidgetRenderer {
	return &colorButtonRenderer{
		button:       c,
		textRenderer: canvas.NewText(c.Text, color.Black),
		iconRenderer: canvas.NewImageFromResource(c.icon),
		bgRenderer: &canvas.Rectangle{
			FillColor:   c.backgroundColor,
			StrokeColor: c.backgroundColor,
		},
	}
}

func (r *colorButtonRenderer) Destroy() {}

func (r *colorButtonRenderer) Layout(size fyne.Size) {
	r.textRenderer.Resize(size)
	r.iconRenderer.Resize(fyne.NewSize(size.Width-4, size.Height-4))
	r.bgRenderer.Resize(size)
}

func (r *colorButtonRenderer) MinSize() fyne.Size {
	textMinSize := r.textRenderer.MinSize()
	return fyne.NewSize(textMinSize.Width-4, textMinSize.Height-4)
}

func (r *colorButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bgRenderer, r.textRenderer, r.iconRenderer}
}

func (r *colorButtonRenderer) Refresh() {
	r.textRenderer.Refresh()
	r.iconRenderer.Refresh()
	r.bgRenderer.Refresh()
}

func (r *colorButtonRenderer) BackgroundColor() color.Color {
	return r.button.backgroundColor
}

// * Custom VBox Layout
type CustomVBoxLayout struct {
	FixedWidth float32
}

func (c *CustomVBoxLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	padding := theme.Padding()

	// The horizontal position is calculated to center the buttons in the space
	x := (size.Width - c.FixedWidth) / 2

	for _, obj := range objects {
		if obj.Visible() {
			obj.Move(fyne.NewPos(x, y))                                  // Center the object horizontally
			obj.Resize(fyne.NewSize(c.FixedWidth, obj.MinSize().Height)) // Set a fixed width for each object
			y += obj.MinSize().Height + padding                          // Increment y position for the next object
		}
	}
}

func (c *CustomVBoxLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	totalHeight := float32(0)
	padding := theme.Padding()

	for _, obj := range objects {
		if obj.Visible() {
			totalHeight += obj.MinSize().Height + padding // Calculate the total height
		}
	}

	if len(objects) > 0 {
		totalHeight -= padding // Remove extra padding after the last element
	}

	return fyne.NewSize(c.FixedWidth, totalHeight) // Return the fixed width for the layout's minimum size
}
