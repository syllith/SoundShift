package fyneCustom

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

type ColorButton struct {
	widget.Button
	backgroundColor color.Color
	icon            fyne.Resource
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

type colorButtonRenderer struct {
	button       *ColorButton
	textRenderer *canvas.Text
	iconRenderer *canvas.Image
	bgRenderer   *canvas.Rectangle
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
