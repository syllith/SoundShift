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
	Disabled   bool // Custom disabled flag
	mouseDown  bool // True while a mouse button is physically held on this widget
	isDragging bool // True while the user is actively dragging the thumb
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
	// Ignore scroll if disabled
	if s.Disabled {
		return
	}
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

// . MouseDown records that the mouse button is held.
// Tracking this ourselves is the key guard against phantom drag events.
func (s *ScrollableSlider) MouseDown(ev *desktop.MouseEvent) {
	s.mouseDown = true
}

// . MouseUp clears both the held-button and dragging flags.
func (s *ScrollableSlider) MouseUp(ev *desktop.MouseEvent) {
	s.mouseDown = false
	s.isDragging = false
}

// . Dragged only forwards drag events to the base slider when the mouse button is
// actually held. Without this guard, Fyne can deliver DragEvent objects that were
// not preceded by a MouseDown on this widget (e.g. after focus changes), causing
// the slider to track raw mouse position with no button pressed.
func (s *ScrollableSlider) Dragged(ev *fyne.DragEvent) {
	if !s.mouseDown {
		return
	}
	s.isDragging = true
	s.Slider.Dragged(ev)
}

// . DragEnd clears the dragging flag and forwards to the base slider.
func (s *ScrollableSlider) DragEnd() {
	s.isDragging = false
	s.Slider.DragEnd()
}

// . IsDragging returns true while the user is actively dragging the slider thumb.
// Use this to suppress external SetValue calls that would interrupt a drag.
func (s *ScrollableSlider) IsDragging() bool {
	return s.isDragging
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
			FillColor:    c.backgroundColor,
			StrokeColor:  c.backgroundColor,
			CornerRadius: 4, // Match Fyne's standard button rounding
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

// . DragArea is a transparent widget that spans the title bar's drag zone.
// It triggers native window dragging when the primary mouse button is pressed.
type DragArea struct {
	widget.BaseWidget
	OnDragStart func()
}

// . NewDragArea creates a DragArea that calls onDragStart on left mouse button press.
func NewDragArea(onDragStart func()) *DragArea {
	d := &DragArea{OnDragStart: onDragStart}
	d.ExtendBaseWidget(d)
	return d
}

func (d *DragArea) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button == desktop.MouseButtonPrimary && d.OnDragStart != nil {
		d.OnDragStart()
	}
}

func (d *DragArea) MouseUp(*desktop.MouseEvent) {}

func (d *DragArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(canvas.NewRectangle(color.Transparent))
}

// titleBarCloseWidth is the pixel width reserved for the close icon on the right.
const titleBarCloseWidth = float32(40)

// TitleBar is a custom title bar widget that renders the app icon, title text, and
// a close icon with a red hover effect. Clicking anywhere except the close icon drags
// the window; clicking the close icon calls onClose.
type TitleBar struct {
	widget.BaseWidget
	icon         fyne.Resource
	title        string
	onClose      func()
	onDragStart  func()
	closeHovered bool
}

type titleBarRenderer struct {
	tb        *TitleBar
	appIcon   *canvas.Image
	titleText *canvas.Text
	closeBg   *canvas.Rectangle
	closeText *canvas.Text
}

// NewTitleBar creates a TitleBar with the given app icon, title, close callback, and drag callback.
func NewTitleBar(icon fyne.Resource, title string, onClose func(), onDragStart func()) *TitleBar {
	tb := &TitleBar{
		icon:        icon,
		title:       title,
		onClose:     onClose,
		onDragStart: onDragStart,
	}
	tb.ExtendBaseWidget(tb)
	return tb
}

func (tb *TitleBar) CreateRenderer() fyne.WidgetRenderer {
	appIcon := canvas.NewImageFromResource(tb.icon)
	appIcon.FillMode = canvas.ImageFillContain

	titleText := canvas.NewText(tb.title, color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff})
	titleText.TextSize = 12

	closeBg := canvas.NewRectangle(color.Transparent)
	closeBg.CornerRadius = 4

	closeText := canvas.NewText("✕", color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xb0})
	closeText.TextSize = 13

	return &titleBarRenderer{
		tb:        tb,
		appIcon:   appIcon,
		titleText: titleText,
		closeBg:   closeBg,
		closeText: closeText,
	}
}

func (r *titleBarRenderer) Layout(size fyne.Size) {
	const iconPad = float32(8)
	const iconSize = float32(14)
	const gap = float32(6)

	// App icon — left edge, vertically centered
	iconY := (size.Height - iconSize) / 2
	r.appIcon.Move(fyne.NewPos(iconPad, iconY))
	r.appIcon.Resize(fyne.NewSize(iconSize, iconSize))

	// Title text — right of icon, vertically centered
	textH := r.titleText.MinSize().Height
	titleX := iconPad + iconSize + gap
	titleY := (size.Height - textH) / 2
	titleW := size.Width - titleX - titleBarCloseWidth
	if titleW < 0 {
		titleW = 0
	}
	r.titleText.Move(fyne.NewPos(titleX, titleY))
	r.titleText.Resize(fyne.NewSize(titleW, textH))

	// Close button background — right edge, full height
	r.closeBg.Move(fyne.NewPos(size.Width-titleBarCloseWidth, 0))
	r.closeBg.Resize(fyne.NewSize(titleBarCloseWidth, size.Height))

	// Close ✕ — centered inside the close button area
	cw := r.closeText.MinSize().Width
	ch := r.closeText.MinSize().Height
	r.closeText.Move(fyne.NewPos(
		size.Width-titleBarCloseWidth+(titleBarCloseWidth-cw)/2,
		(size.Height-ch)/2,
	))
	r.closeText.Resize(fyne.NewSize(cw, ch))
}

func (r *titleBarRenderer) MinSize() fyne.Size {
	return fyne.NewSize(titleBarCloseWidth+100, 34)
}

func (r *titleBarRenderer) Refresh() {
	if r.tb.closeHovered {
		r.closeBg.FillColor = color.NRGBA{R: 0xc4, G: 0x2b, B: 0x1c, A: 0xcc}
		r.closeText.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
	} else {
		r.closeBg.FillColor = color.Transparent
		r.closeText.Color = color.NRGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xb0}
	}
	r.closeBg.Refresh()
	r.closeText.Refresh()
}

func (r *titleBarRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.appIcon, r.titleText, r.closeBg, r.closeText}
}

func (r *titleBarRenderer) Destroy() {}

// Mouse drag — fires on the non-close area of the title bar
func (tb *TitleBar) MouseDown(ev *desktop.MouseEvent) {
	if ev.Button == desktop.MouseButtonPrimary && ev.Position.X < tb.Size().Width-titleBarCloseWidth {
		if tb.onDragStart != nil {
			tb.onDragStart()
		}
	}
}

// Close click — fires when the mouse is released over the close icon area
func (tb *TitleBar) MouseUp(ev *desktop.MouseEvent) {
	if ev.Button == desktop.MouseButtonPrimary && ev.Position.X >= tb.Size().Width-titleBarCloseWidth {
		if tb.onClose != nil {
			tb.onClose()
		}
	}
}

// Hover tracking for close icon highlight
func (tb *TitleBar) MouseIn(*desktop.MouseEvent) {}

func (tb *TitleBar) MouseMoved(ev *desktop.MouseEvent) {
	hovered := ev.Position.X >= tb.Size().Width-titleBarCloseWidth
	if hovered != tb.closeHovered {
		tb.closeHovered = hovered
		tb.Refresh()
	}
}

func (tb *TitleBar) MouseOut() {
	if tb.closeHovered {
		tb.closeHovered = false
		tb.Refresh()
	}
}
