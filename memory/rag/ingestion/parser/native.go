package parser

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/BurntSushi/toml"
	"golang.org/x/net/html"
	"gopkg.in/yaml.v3"
)

type NativeParser struct{}

func NewNativeParser() *NativeParser {
	return &NativeParser{}
}

func (np *NativeParser) SupportedFormats() []string {
	return []string{"txt", "md", "markdown", "html", "htm", "csv", "json", "xml", "log",
		"yaml", "yml", "toml", "ini", "cfg", "conf", "env"}
}

func (np *NativeParser) Parse(reader io.Reader, opts ParseOptions) (*Document, error) {
	r := io.LimitReader(reader, opts.MaxSize+1)
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > opts.MaxSize {
		return nil, ErrDocumentTooLarge
	}
	if len(data) == 0 {
		return nil, ErrEmptyContent
	}

	if opts.Encoding == "" {
		opts.Encoding = "utf-8"
	}
	opts.Encoding = strings.ToLower(opts.Encoding)
	if opts.Encoding != "utf-8" && opts.Encoding != "utf8" {
		return nil, fmt.Errorf("%w: only UTF-8 is supported", ErrParseFailed)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%w: content is not valid UTF-8", ErrParseFailed)
	}

	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(opts.Filename), "."))
	if ext == "" {
		ext = "txt"
	}

	content, err := convertToMarkdown(data, ext)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrParseFailed, err)
	}

	return &Document{
		Format:   ext,
		Content:  content,
		Size:     int64(len(data)),
		Metadata: make(map[string]string),
	}, nil
}

func convertToMarkdown(data []byte, ext string) (string, error) {
	switch ext {
	case "csv":
		return csvToMarkdown(data)
	case "json":
		return jsonToMarkdown(data)
	case "xml":
		return xmlToMarkdown(data)
	case "html", "htm":
		return htmlToMarkdown(data)
	case "yaml", "yml":
		return yamlToStructuredMarkdown(data)
	case "toml":
		return tomlToStructuredMarkdown(data)
	default:
		return string(data), nil
	}
}

func csvToMarkdown(data []byte) (string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.LazyQuotes = true
	r.FieldsPerRecord = -1

	records, err := r.ReadAll()
	if err != nil {
		return "", fmt.Errorf("csv: %w", err)
	}
	if len(records) == 0 {
		return "", nil
	}

	maxCols := 0
	for _, row := range records {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	var sb strings.Builder
	for i, row := range records {
		for len(row) < maxCols {
			row = append(row, "")
		}
		for j, cell := range row {
			row[j] = strings.ReplaceAll(cell, "\n", "<br>")
		}
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
		if i == 0 {
			sb.WriteString("|" + strings.Repeat("---|", maxCols) + "\n")
		}
	}
	return sb.String(), nil
}

func jsonToMarkdown(data []byte) (string, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("json: %w", err)
	}
	var sb strings.Builder
	walkStructured(&sb, v, 0, "")
	return cleanupMarkdown(sb.String()), nil
}

func yamlToStructuredMarkdown(data []byte) (string, error) {
	var v interface{}
	if err := yaml.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("yaml: %w", err)
	}
	var sb strings.Builder
	walkStructured(&sb, v, 0, "")
	return cleanupMarkdown(sb.String()), nil
}

func tomlToStructuredMarkdown(data []byte) (string, error) {
	var v interface{}
	if err := toml.Unmarshal(data, &v); err != nil {
		return "", fmt.Errorf("toml: %w", err)
	}
	var sb strings.Builder
	walkStructured(&sb, v, 0, "")
	return cleanupMarkdown(sb.String()), nil
}

func walkStructured(sb *strings.Builder, v interface{}, depth int, key string) {
	headingLevel := depth + 1
	if headingLevel > 6 {
		headingLevel = 6
	}
	headingPrefix := strings.Repeat("#", headingLevel)

	switch val := v.(type) {
	case map[string]interface{}:
		if len(val) == 0 {
			if key != "" {
				sb.WriteString(fmt.Sprintf("- **%s**: (empty)\n", key))
			}
			return
		}
		if key != "" {
			sb.WriteString("\n" + headingPrefix + " " + key + "\n\n")
		}
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if isScalar(val[k]) {
				walkStructured(sb, val[k], depth+1, k)
			}
		}
		for _, k := range keys {
			if !isScalar(val[k]) {
				walkStructured(sb, val[k], depth+1, k)
			}
		}

	case []interface{}:
		if len(val) == 0 {
			if key != "" {
				sb.WriteString(fmt.Sprintf("- **%s**: (empty)\n", key))
			}
			return
		}
		if key != "" {
			sb.WriteString("\n" + headingPrefix + " " + key + "\n\n")
		}
		for i, item := range val {
			switch item.(type) {
			case map[string]interface{}:
				sb.WriteString("\n")
				walkStructured(sb, item, depth, "")
			default:
				if isScalar(item) {
					sb.WriteString(fmt.Sprintf("%d. %v\n", i+1, item))
				} else {
					sb.WriteString(fmt.Sprintf("%d. ", i+1))
					walkStructured(sb, item, depth, "")
				}
			}
		}

	default:
		if key != "" {
			sb.WriteString(fmt.Sprintf("- **%s**: %v\n", key, val))
		} else {
			sb.WriteString(fmt.Sprintf("%v\n", val))
		}
	}
}

func isScalar(v interface{}) bool {
	switch v.(type) {
	case map[string]interface{}, []interface{}:
		return false
	}
	return true
}

type xmlNode struct {
	Name     string
	Attrs    map[string]string
	Text     string
	Children []xmlNode
}

func stripNS(name string) string {
	if i := strings.IndexByte(name, ':'); i >= 0 {
		return name[i+1:]
	}
	return name
}

func xmlToMarkdown(data []byte) (string, error) {
	root, err := buildXMLTree(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("xml: %w", err)
	}
	if root == nil {
		return "", nil
	}
	var sb strings.Builder
	renderXMLNode(&sb, root, 0)
	return cleanupMarkdown(sb.String()), nil
}

func buildXMLTree(r io.Reader) (*xmlNode, error) {
	decoder := xml.NewDecoder(r)
	decoder.Strict = false
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		if start, ok := tok.(xml.StartElement); ok {
			node, err := parseXMLElement(decoder, start)
			if err != nil {
				return nil, err
			}
			return &node, nil
		}
	}
}

func parseXMLElement(decoder *xml.Decoder, start xml.StartElement) (xmlNode, error) {
	node := xmlNode{
		Name:  stripNS(start.Name.Local),
		Attrs: make(map[string]string),
	}
	for _, a := range start.Attr {
		node.Attrs[stripNS(a.Name.Local)] = a.Value
	}

	var textParts []string
	for {
		tok, err := decoder.Token()
		if err != nil {
			return xmlNode{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			child, err := parseXMLElement(decoder, t)
			if err != nil {
				return xmlNode{}, err
			}
			node.Children = append(node.Children, child)
		case xml.EndElement:
			node.Text = strings.Join(textParts, " ")
			return node, nil
		case xml.CharData:
			text := strings.TrimSpace(string(t))
			if text != "" {
				textParts = append(textParts, text)
			}
		}
	}
}

func renderXMLNode(sb *strings.Builder, node *xmlNode, depth int) {
	hLevel := depth + 1
	if hLevel > 6 {
		hLevel = 6
	}
	prefix := strings.Repeat("#", hLevel)

	// Root 行为：标题 + 属性 + 如果只有一个同名单层子节点则下沉
	if depth == 0 {
		sb.WriteString("# " + node.Name)
		if len(node.Attrs) > 0 {
			sb.WriteString(" (" + formatAttrsKV(node.Attrs) + ")")
		}
		sb.WriteString("\n\n")
	}

	// 叶子节点：由父级渲染
	if len(node.Children) == 0 {
		return
	}

	// 非 root 写标题
	if depth > 0 {
		sb.WriteString("\n" + prefix + " " + node.Name)
		if len(node.Attrs) > 0 {
			sb.WriteString(" (" + formatAttrsKV(node.Attrs) + ")")
		}
		sb.WriteString("\n\n")
	}

	// 节点自身的文本（混合内容）
	if node.Text != "" {
		sb.WriteString(node.Text + "\n\n")
	}

	// 按名称分组子节点
	groups := make(map[string][]*xmlNode)
	var order []string
	for i := range node.Children {
		name := node.Children[i].Name
		if _, seen := groups[name]; !seen {
			order = append(order, name)
		}
		groups[name] = append(groups[name], &node.Children[i])
	}
	sort.Strings(order)

	// 先叶子后嵌套（与 JSON/YAML 一致）
	type groupEntry struct {
		name  string
		items []*xmlNode
	}
	var leafGroups, nestedGroups []groupEntry
	for _, name := range order {
		items := groups[name]
		isLeaf := (len(items) == 1 && len(items[0].Children) == 0) || (len(items) >= 2 && itemsAreLeaves(items))
		if isLeaf {
			leafGroups = append(leafGroups, groupEntry{name, items})
		} else {
			nestedGroups = append(nestedGroups, groupEntry{name, items})
		}
	}

	for _, g := range leafGroups {
		for _, item := range g.items {
			renderLeafKV(sb, item)
		}
	}

	for _, g := range nestedGroups {
		if len(g.items) >= 2 {
			if canBeTable(g.items) {
				renderXMLTable(sb, g.items)
				continue
			}
			renderXMLList(sb, g.name, g.items, depth+1)
			continue
		}
		renderXMLNode(sb, g.items[0], depth+1)
	}
}

func renderLeafKV(sb *strings.Builder, node *xmlNode) {
	text := node.Text
	if text == "" && len(node.Attrs) > 0 {
		text = formatAttrsKV(node.Attrs)
	} else if text == "" {
		text = "(empty)"
	}
	line := fmt.Sprintf("- **%s**: %s", node.Name, text)
	if len(node.Attrs) > 0 && node.Text != "" {
		line += fmt.Sprintf(" (%s)", formatAttrsKV(node.Attrs))
	}
	sb.WriteString(line + "\n")
}

func renderXMLTable(sb *strings.Builder, items []*xmlNode) {
	cols := getLeafNames(items[0])
	if len(cols) == 0 {
		return
	}
	sb.WriteString("\n| " + strings.Join(cols, " | ") + " |\n")
	sb.WriteString("|" + strings.Repeat("---|", len(cols)) + "\n")

	for _, item := range items {
		sb.WriteString("| ")
		for _, col := range cols {
			val := ""
			for _, c := range item.Children {
				if c.Name == col {
					val = c.Text
					if len(c.Attrs) > 0 {
						val += " (" + formatAttrsKV(c.Attrs) + ")"
					}
					break
				}
			}
			sb.WriteString(val + " | ")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

func itemsAreLeaves(items []*xmlNode) bool {
	for _, item := range items {
		if len(item.Children) > 0 {
			return false
		}
	}
	return true
}

func allLeaves(items []*xmlNode) bool {
	for _, item := range items {
		for _, c := range item.Children {
			if len(c.Children) > 0 {
				return false
			}
		}
	}
	return true
}

func renderXMLList(sb *strings.Builder, name string, items []*xmlNode, depth int) {
	hasComplex := false
	for _, item := range items {
		for _, c := range item.Children {
			if len(c.Children) > 0 {
				hasComplex = true
				break
			}
		}
	}

	if hasComplex {
		// 复杂项 → 每项作为子 section
		for _, item := range items {
			renderXMLNode(sb, item, depth)
		}
		return
	}

	// 简单项 → 每项一行，children 内联
	sb.WriteString("\n")
	for i, item := range items {
		sb.WriteString(fmt.Sprintf("%d. ", i+1))
		var parts []string
		for _, c := range item.Children {
			t := c.Text
			if t == "" && len(c.Attrs) > 0 {
				t = formatAttrsKV(c.Attrs)
			}
			parts = append(parts, fmt.Sprintf("%s: %s", c.Name, t))
		}
		if item.Text != "" {
			sb.WriteString(item.Text)
			if len(parts) > 0 {
				sb.WriteString(" — ")
			}
		}
		sb.WriteString(strings.Join(parts, ", "))
		if len(item.Attrs) > 0 {
			sb.WriteString(" (" + formatAttrsKV(item.Attrs) + ")")
		}
		sb.WriteString("\n")
	}
	sb.WriteString("\n")
}

func canBeTable(items []*xmlNode) bool {
	if len(items) < 2 || !allLeaves(items) {
		return false
	}
	cols := getLeafNames(items[0])
	if len(cols) == 0 {
		return false
	}
	for i := 1; i < len(items); i++ {
		itemCols := getLeafNames(items[i])
		if len(itemCols) != len(cols) {
			return false
		}
		for j, name := range cols {
			if itemCols[j] != name {
				return false
			}
		}
	}
	return true
}

func getLeafNames(node *xmlNode) []string {
	var names []string
	for _, c := range node.Children {
		if len(c.Children) == 0 {
			names = append(names, c.Name)
		}
	}
	return names
}

func formatAttrsKV(attrs map[string]string) string {
	if len(attrs) == 0 {
		return ""
	}
	keys := make([]string, 0, len(attrs))
	for k := range attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + attrs[k]
	}
	return strings.Join(parts, ", ")
}

func htmlToMarkdown(data []byte) (string, error) {
	doc, err := html.Parse(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("html: %w", err)
	}
	var sb strings.Builder
	walkHTML(&sb, doc)
	return cleanupMarkdown(sb.String()), nil
}

func walkHTML(sb *strings.Builder, n *html.Node) {
	if n.Type == html.TextNode {
		text := strings.TrimSpace(n.Data)
		if text != "" {
			if sb.Len() > 0 && needsWordBreak(sb.String()[sb.Len()-1]) {
				sb.WriteString(" ")
			}
			sb.WriteString(text)
		}
		return
	}

	if n.Type != html.ElementNode {
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkHTML(sb, c)
		}
		return
	}

	switch n.Data {
	case "script", "style", "noscript", "head", "title", "meta", "link":
		return

	case "h1", "h2", "h3", "h4", "h5", "h6":
		level := int(n.Data[1] - '0')
		sb.WriteString("\n" + strings.Repeat("#", level) + " ")
		walkChildren(sb, n)
		sb.WriteString("\n")

	case "p", "div", "section", "article":
		sb.WriteString("\n")
		walkChildren(sb, n)
		sb.WriteString("\n")

	case "br", "hr":
		sb.WriteString("\n")

	case "strong", "b":
		sb.WriteString("**")
		walkChildren(sb, n)
		sb.WriteString("**")

	case "em", "i":
		sb.WriteString("*")
		walkChildren(sb, n)
		sb.WriteString("*")

	case "code":
		if n.Parent != nil && n.Parent.Data == "pre" {
			walkChildren(sb, n)
		} else if parentIsBlock(n) {
			sb.WriteString("\n```\n")
			walkChildren(sb, n)
			sb.WriteString("\n```\n")
		} else {
			sb.WriteString("`")
			walkChildren(sb, n)
			sb.WriteString("`")
		}

	case "pre":
		sb.WriteString("\n```\n")
		walkChildren(sb, n)
		sb.WriteString("\n```\n")

	case "a":
		href := getAttr(n, "href")
		sb.WriteString("[")
		walkChildren(sb, n)
		sb.WriteString("](" + href + ")")

	case "img":
		alt := getAttr(n, "alt")
		src := getAttr(n, "src")
		sb.WriteString("![" + alt + "](" + src + ")")

	case "ul":
		sb.WriteString("\n")
		walkListItems(sb, n, false)
		sb.WriteString("\n")

	case "ol":
		sb.WriteString("\n")
		walkListItems(sb, n, true)
		sb.WriteString("\n")

	case "li":
		walkChildren(sb, n)

	case "table":
		walkTable(sb, n)
		sb.WriteString("\n")

	case "blockquote":
		sb.WriteString("\n> ")
		walkChildren(sb, n)
		sb.WriteString("\n")

	default:
		walkChildren(sb, n)
	}
}

func walkChildren(sb *strings.Builder, n *html.Node) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walkHTML(sb, c)
	}
}

func walkListItems(sb *strings.Builder, n *html.Node, ordered bool) {
	count := 1
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == "li" {
			if ordered {
				sb.WriteString(fmt.Sprintf("%d. ", count))
				count++
			} else {
				sb.WriteString("- ")
			}
			walkChildren(sb, c)
			sb.WriteString("\n")
		}
	}
}

func walkTable(sb *strings.Builder, n *html.Node) {
	var rows [][]string
	collectRows(&rows, n)

	if len(rows) == 0 {
		return
	}

	maxCols := 0
	for _, row := range rows {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	sb.WriteString("\n")
	for i, row := range rows {
		for len(row) < maxCols {
			row = append(row, "")
		}
		sb.WriteString("| " + strings.Join(row, " | ") + " |\n")
		if i == 0 {
			sb.WriteString("|" + strings.Repeat("---|", maxCols) + "\n")
		}
	}
}

func collectRows(rows *[][]string, n *html.Node) {
	if n.Type == html.ElementNode {
		if n.Data == "thead" || n.Data == "tbody" || n.Data == "tfoot" {
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				collectRows(rows, c)
			}
			return
		}
		if n.Data == "tr" {
			var cells []string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && (c.Data == "td" || c.Data == "th") {
					var cellText strings.Builder
					walkChildren(&cellText, c)
					cells = append(cells, strings.TrimSpace(strings.ReplaceAll(cellText.String(), "\n", " ")))
				}
			}
			if len(cells) > 0 {
				*rows = append(*rows, cells)
			}
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		collectRows(rows, c)
	}
}

func parentIsBlock(n *html.Node) bool {
	p := n.Parent
	if p == nil {
		return false
	}
	switch p.Data {
	case "pre", "p", "div", "section", "article", "blockquote", "li", "td", "th":
		return false
	}
	return true
}

func getAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

func needsWordBreak(last byte) bool {
	return (last >= 'a' && last <= 'z') || (last >= 'A' && last <= 'Z') || (last >= '0' && last <= '9')
}

func cleanupMarkdown(s string) string {
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}
