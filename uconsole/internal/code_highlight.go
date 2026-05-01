//go:build uconsole_gui

package uconsole

import (
	"image/color"
	"strings"
	"unicode"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

var commonShellCommands = map[string]bool{
	"bash": true, "sh": true, "zsh": true, "fish": true, "env": true, "go": true, "git": true, "rg": true, "sed": true,
	"awk": true, "jq": true, "curl": true, "wget": true, "tmux": true, "cat": true, "ls": true, "cd": true, "cp": true,
	"mv": true, "rm": true, "mkdir": true, "chmod": true, "chown": true, "systemctl": true, "journalctl": true, "python": true,
	"python3": true, "node": true, "npm": true, "pnpm": true, "yarn": true, "make": true, "docker": true, "kubectl": true,
}

type codeSpan struct {
	Text      string
	Color     color.NRGBA
	TextStyle fyne.TextStyle
}

type codeParagraph struct {
	widget.BaseWidget
	spans []codeSpan
}

type codeParagraphFragment struct {
	Text      string
	Color     color.NRGBA
	TextStyle fyne.TextStyle
	X         float32
	Y         float32
}

type codeParagraphRenderer struct {
	paragraph *codeParagraph
	fragments []codeParagraphFragment
	objects   []fyne.CanvasObject
	width     float32
	height    float32
}

func newCodeParagraph(spans []codeSpan) *codeParagraph {
	paragraph := &codeParagraph{spans: compactCodeSpans(spans)}
	paragraph.ExtendBaseWidget(paragraph)
	return paragraph
}

func (p *codeParagraph) CreateRenderer() fyne.WidgetRenderer {
	return &codeParagraphRenderer{paragraph: p}
}

func (r *codeParagraphRenderer) Layout(size fyne.Size) {
	width := size.Width
	if width <= 0 {
		width = markdownParagraphLayoutWidth(0)
	}
	r.ensureLayout(width)

	for i, object := range r.objects {
		if i >= len(r.fragments) {
			object.Hide()
			continue
		}
		text := object.(*canvas.Text)
		fragment := r.fragments[i]
		text.Text = fragment.Text
		text.Color = fragment.Color
		text.TextStyle = fragment.TextStyle
		text.TextSize = codeTextSize()
		text.Move(fyne.NewPos(fragment.X, fragment.Y))
		text.Resize(text.MinSize())
		text.Show()
		text.Refresh()
	}
}

func (r *codeParagraphRenderer) MinSize() fyne.Size {
	width := markdownParagraphLayoutWidth(r.paragraph.Size().Width)
	r.ensureLayout(width)
	return fyne.NewSize(width, r.height)
}

func (r *codeParagraphRenderer) Refresh() {
	r.width = 0
	r.ensureLayout(r.paragraph.Size().Width)
	for _, object := range r.objects {
		object.Refresh()
	}
}

func (r *codeParagraphRenderer) Objects() []fyne.CanvasObject {
	r.ensureLayout(r.paragraph.Size().Width)
	return r.objects
}

func (r *codeParagraphRenderer) Destroy() {}

func (r *codeParagraphRenderer) ensureLayout(width float32) {
	width = markdownParagraphLayoutWidth(width)
	if r.width == width {
		return
	}

	fragments, height := layoutCodeParagraph(r.paragraph.spans, width)
	r.fragments = fragments
	r.height = height
	r.width = width

	r.objects = make([]fyne.CanvasObject, 0, len(fragments))
	for _, fragment := range fragments {
		text := canvas.NewText(fragment.Text, fragment.Color)
		text.TextStyle = fragment.TextStyle
		text.TextSize = codeTextSize()
		r.objects = append(r.objects, text)
	}
}

func compactCodeSpans(spans []codeSpan) []codeSpan {
	merged := make([]codeSpan, 0, len(spans))
	for _, span := range spans {
		if span.Text == "" {
			continue
		}
		lastIndex := len(merged) - 1
		if lastIndex >= 0 {
			last := merged[lastIndex]
			if last.Color == span.Color && last.TextStyle == span.TextStyle {
				merged[lastIndex].Text += span.Text
				continue
			}
		}
		merged = append(merged, span)
	}
	return merged
}

func layoutCodeParagraph(spans []codeSpan, width float32) ([]codeParagraphFragment, float32) {
	width = markdownParagraphLayoutWidth(width)

	tokens := codeTokens(spans)
	if len(tokens) == 0 {
		return nil, codeLineHeight()
	}

	lineSpacing := fyne.CurrentApp().Settings().Theme().Size(theme.SizeNameLineSpacing)
	fragments := make([]codeParagraphFragment, 0, len(tokens))
	x := float32(0)
	y := float32(0)
	rowHeight := float32(0)

	newLine := func() {
		if rowHeight <= 0 {
			rowHeight = codeLineHeight()
		}
		x = 0
		y += rowHeight + lineSpacing
		rowHeight = 0
	}

	appendFragment := func(text string, token codeSpan) {
		if text == "" {
			return
		}
		size := fyne.MeasureText(text, codeTextSize(), token.TextStyle)
		if size.Height > rowHeight {
			rowHeight = size.Height
		}
		lastIndex := len(fragments) - 1
		if lastIndex >= 0 {
			last := &fragments[lastIndex]
			if last.Color == token.Color && last.TextStyle == token.TextStyle && last.Y == y &&
				last.X+fyne.MeasureText(last.Text, codeTextSize(), last.TextStyle).Width == x {
				last.Text += text
				x += size.Width
				return
			}
		}
		fragments = append(fragments, codeParagraphFragment{
			Text:      text,
			Color:     token.Color,
			TextStyle: token.TextStyle,
			X:         x,
			Y:         y,
		})
		x += size.Width
	}

	for _, token := range tokens {
		if token.Text == "\n" {
			newLine()
			continue
		}
		text := token.Text
		for text != "" {
			size := fyne.MeasureText(text, codeTextSize(), token.TextStyle)
			if size.Width <= width-x {
				appendFragment(text, token)
				break
			}

			part, rest := splitCodeToken(text, token.TextStyle, width-x)
			if part == "" {
				if x > 0 {
					newLine()
					continue
				}
				runes := []rune(text)
				part = string(runes[:1])
				rest = string(runes[1:])
			}
			appendFragment(part, token)
			text = rest
			if text != "" {
				newLine()
			}
		}
	}

	if rowHeight <= 0 {
		rowHeight = codeLineHeight()
	}
	return fragments, y + rowHeight
}

func codeTokens(spans []codeSpan) []codeSpan {
	tokens := make([]codeSpan, 0, len(spans)*4)
	for _, span := range spans {
		for _, token := range splitCodeText(span.Text) {
			tokens = append(tokens, codeSpan{
				Text:      token,
				Color:     span.Color,
				TextStyle: span.TextStyle,
			})
		}
	}
	return tokens
}

func splitCodeText(text string) []string {
	tokens := make([]string, 0, len(text))
	runes := []rune(text)
	for i := 0; i < len(runes); {
		r := runes[i]
		switch {
		case r == '\n':
			tokens = append(tokens, "\n")
			i++
		case unicode.IsSpace(r):
			start := i
			for i < len(runes) && unicode.IsSpace(runes[i]) && runes[i] != '\n' {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
		default:
			start := i
			for i < len(runes) && !unicode.IsSpace(runes[i]) {
				i++
			}
			tokens = append(tokens, string(runes[start:i]))
		}
	}
	return tokens
}

func splitCodeToken(text string, textStyle fyne.TextStyle, width float32) (string, string) {
	if width <= 0 {
		return "", text
	}

	runes := []rune(text)
	if len(runes) <= 1 {
		return "", text
	}

	lastFit := 0
	for i := 1; i <= len(runes); i++ {
		part := string(runes[:i])
		if fyne.MeasureText(part, codeTextSize(), textStyle).Width <= width {
			lastFit = i
			continue
		}
		break
	}
	if lastFit == 0 {
		return "", text
	}
	return string(runes[:lastFit]), string(runes[lastFit:])
}

func codeTextSize() float32 {
	return fyne.CurrentApp().Settings().Theme().Size(theme.SizeNameText)
}

func codeLineHeight() float32 {
	return fyne.MeasureText("Mg", codeTextSize(), fyne.TextStyle{Monospace: true}).Height
}

func newHighlightedCodeBlockObject(text string, darkMode bool) fyne.CanvasObject {
	lines := buildHighlightedCodeLines(text, darkMode)
	contentItems := make([]fyne.CanvasObject, 0, len(lines)*2)
	for i, line := range lines {
		contentItems = append(contentItems, newCodeParagraph(line))
		if i < len(lines)-1 {
			gap := canvas.NewRectangle(color.Transparent)
			gap.SetMinSize(fyne.NewSize(1, codeLineHeight()*0.2))
			contentItems = append(contentItems, gap)
		}
	}
	if len(contentItems) == 0 {
		contentItems = append(contentItems, newCodeParagraph(nil))
	}

	return container.NewPadded(container.New(layout.NewCustomPaddedVBoxLayout(0), contentItems...))
}

func buildHighlightedCodeLines(text string, darkMode bool) [][]codeSpan {
	iterator, style := highlightedCodeIterator(text, darkMode, false)

	lines := make([][]codeSpan, 0, 8)
	current := make([]codeSpan, 0, 8)
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := style.Get(token.Type)
		spanColor := chromaColourToNRGBA(entry.Colour, darkMode)
		textStyle := fyne.TextStyle{
			Monospace: true,
			Bold:      entry.Bold == chroma.Yes,
			Italic:    entry.Italic == chroma.Yes,
		}

		parts := strings.SplitAfter(token.Value, "\n")
		for _, part := range parts {
			if part == "" {
				continue
			}
			lineBreak := strings.HasSuffix(part, "\n")
			part = strings.TrimSuffix(part, "\n")
			if part != "" {
				current = append(current, codeSpan{
					Text:      part,
					Color:     spanColor,
					TextStyle: textStyle,
				})
			}
			if lineBreak {
				lines = append(lines, compactCodeSpans(current))
				current = make([]codeSpan, 0, 8)
			}
		}
	}
	if len(current) > 0 || len(lines) == 0 {
		lines = append(lines, compactCodeSpans(current))
	}
	return lines
}

func highlightedInlineCodeSpans(text string, darkMode bool) []markdownSpan {
	style := markdownCodeInlineStyle()
	if strings.TrimSpace(text) == "" {
		return []markdownSpan{{Text: text, Style: style}}
	}
	if looksLikePath(text) {
		return compactMarkdownSpans(highlightedInlinePathSpans(text, style, darkMode))
	}

	iterator, chromaStyle := highlightedCodeIterator(text, darkMode, true)
	out := make([]markdownSpan, 0, 8)
	for token := iterator(); token != chroma.EOF; token = iterator() {
		entry := chromaStyle.Get(token.Type)
		out = append(out, markdownSpan{
			Text: token.Value,
			Style: widget.RichTextStyle{
				Inline:    true,
				SizeName:  theme.SizeNameText,
				TextStyle: fyne.TextStyle{Monospace: true, Bold: entry.Bold == chroma.Yes, Italic: entry.Italic == chroma.Yes},
			},
			Color:    chromaColourToNRGBA(entry.Colour, darkMode),
			HasColor: true,
		})
	}
	return compactMarkdownSpans(out)
}

func highlightedCodeIterator(text string, darkMode bool, inline bool) (chroma.Iterator, *chroma.Style) {
	lexer := chooseHighlightLexer(text, inline)
	iterator, err := lexer.Tokenise(nil, text)
	if err != nil {
		iterator = chroma.Literator(chroma.Token{Type: chroma.Text, Value: text})
	}

	styleName := "github"
	if darkMode {
		styleName = "github-dark"
	}
	return iterator, styles.Get(styleName)
}

func chooseHighlightLexer(text string, inline bool) chroma.Lexer {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return lexers.Fallback
	}
	if looksLikeShellSnippet(trimmed) {
		if lexer := lexers.Get("bash"); lexer != nil {
			return lexer
		}
	}
	if lexer := lexers.Match(trimmed); lexer != nil && lexer != lexers.Fallback {
		return lexer
	}
	if lexer := lexers.Analyse(trimmed); lexer != nil {
		return lexer
	}
	if inline && looksLikePath(trimmed) {
		return lexers.Fallback
	}
	return lexers.Fallback
}

func looksLikePath(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || strings.Contains(trimmed, "\n") {
		return false
	}
	if strings.HasPrefix(trimmed, "/") || strings.HasPrefix(trimmed, "./") || strings.HasPrefix(trimmed, "../") || strings.HasPrefix(trimmed, "~/") {
		return true
	}
	if strings.Contains(trimmed, "/") && !strings.Contains(trimmed, " ") {
		return true
	}
	if strings.HasSuffix(trimmed, ".go") || strings.HasSuffix(trimmed, ".md") || strings.HasSuffix(trimmed, ".json") || strings.HasSuffix(trimmed, ".yaml") || strings.HasSuffix(trimmed, ".yml") || strings.HasSuffix(trimmed, ".sh") || strings.HasSuffix(trimmed, ".toml") {
		return true
	}
	return false
}

func looksLikeShellSnippet(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "$ ") || strings.HasPrefix(trimmed, "# ") {
		return true
	}
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return false
	}
	head := fields[0]
	if head == "sudo" && len(fields) > 1 {
		head = fields[1]
	}
	head = strings.TrimSpace(head)
	if commonShellCommands[head] {
		return true
	}
	if strings.HasPrefix(head, "./") || strings.HasPrefix(head, "../") {
		return true
	}
	for _, marker := range []string{" --", " -", " | ", " && ", " || ", " > ", " < ", "$(", "${"} {
		if strings.Contains(trimmed, marker) {
			return true
		}
	}
	return false
}

func inlinePathColor(darkMode bool) color.NRGBA {
	if darkMode {
		return color.NRGBA{R: 0x7E, G: 0xC6, B: 0xFF, A: 0xFF}
	}
	return color.NRGBA{R: 0x05, G: 0x5D, B: 0xA6, A: 0xFF}
}

func highlightedInlinePathSpans(text string, style widget.RichTextStyle, darkMode bool) []markdownSpan {
	dirColor, baseColor, extColor := inlinePathPalette(darkMode)
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return []markdownSpan{{Text: text, Style: style}}
	}

	lastSlash := strings.LastIndex(trimmed, "/")
	lastDot := strings.LastIndex(trimmed, ".")
	hasExt := lastDot > lastSlash && lastDot > 0 && lastDot < len(trimmed)-1

	out := make([]markdownSpan, 0, 4)
	if lastSlash >= 0 {
		dir := trimmed[:lastSlash+1]
		if dir != "" {
			out = append(out, markdownSpan{
				Text:     dir,
				Style:    style,
				Color:    dirColor,
				HasColor: true,
			})
		}
		trimmed = trimmed[lastSlash+1:]
	}

	baseEnd := len(trimmed)
	if hasExt {
		baseEnd = lastDot - lastSlash - 1
	}
	if baseEnd > 0 {
		out = append(out, markdownSpan{
			Text:     trimmed[:baseEnd],
			Style:    style,
			Color:    baseColor,
			HasColor: true,
		})
	}
	if hasExt {
		out = append(out, markdownSpan{
			Text:     trimmed[baseEnd:],
			Style:    style,
			Color:    extColor,
			HasColor: true,
		})
	}
	if len(out) == 0 {
		return []markdownSpan{{
			Text:     text,
			Style:    style,
			Color:    inlinePathColor(darkMode),
			HasColor: true,
		}}
	}
	return out
}

func inlinePathPalette(darkMode bool) (dir color.NRGBA, base color.NRGBA, ext color.NRGBA) {
	if darkMode {
		return color.NRGBA{R: 0x7E, G: 0xC6, B: 0xFF, A: 0xFF},
			color.NRGBA{R: 0xE6, G: 0xED, B: 0xF3, A: 0xFF},
			color.NRGBA{R: 0x79, G: 0xC0, B: 0xFF, A: 0xFF}
	}
	return color.NRGBA{R: 0x05, G: 0x5D, B: 0xA6, A: 0xFF},
		color.NRGBA{R: 0x24, G: 0x2C, B: 0x36, A: 0xFF},
		color.NRGBA{R: 0x82, G: 0x50, B: 0xB2, A: 0xFF}
}

func chromaColourToNRGBA(value chroma.Colour, darkMode bool) color.NRGBA {
	if value.IsSet() {
		return color.NRGBA{
			R: value.Red(),
			G: value.Green(),
			B: value.Blue(),
			A: 0xFF,
		}
	}
	if darkMode {
		return color.NRGBA{R: 0xE6, G: 0xED, B: 0xF3, A: 0xFF}
	}
	return color.NRGBA{R: 0x24, G: 0x2C, B: 0x36, A: 0xFF}
}
