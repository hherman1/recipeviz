// Package recipe parses, validates, and renders recipe dependency forests.
package recipe

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"unicode/utf8"
)

// AST preserves source order while separating each step's result name,
// display text, and referenced results.
type AST struct {
	Steps []Step
}

// Step is one parsed recipe line.
type Step struct {
	Line   int
	Result string
	Text   string
	Inputs []string
}

// Forest contains the roots of independent recipe dependency trees.
type Forest struct {
	Roots []*Node
}

// Node is a rendered step whose children are the results it incorporates.
type Node struct {
	Text       string
	SourceLine int
	Children   []*Node
}

// Parse recognizes result declarations first so ordinary words at the end of
// a step are not mistaken for references. Blank lines are ignored.
func Parse(source string) (AST, error) {
	var ast AST
	declared := make(map[string]int)

	for i, rawLine := range strings.Split(source, "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		result, text := "", line
		if colon := strings.Index(line, ": "); colon > 0 {
			name := line[:colon]
			validName := true
			for _, r := range name {
				if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
					validName = false
					break
				}
			}
			if validName {
				if previousLine, exists := declared[name]; exists {
					return AST{}, fmt.Errorf("line %d: result %q was already declared on line %d", i+1, name, previousLine)
				}
				result = name
				text = line[colon+2:]
				declared[name] = i + 1
			}
		}
		if strings.TrimSpace(text) == "" {
			return AST{}, fmt.Errorf("line %d: step has no description", i+1)
		}
		ast.Steps = append(ast.Steps, Step{
			Line:   i + 1,
			Result: result,
			Text:   text,
		})
	}
	if len(ast.Steps) == 0 {
		return AST{}, fmt.Errorf("recipe has no steps")
	}

	for i := range ast.Steps {
		fields := strings.Fields(ast.Steps[i].Text)
		firstInput := len(fields)
		for firstInput > 0 {
			if _, exists := declared[fields[firstInput-1]]; !exists {
				break
			}
			firstInput--
		}
		ast.Steps[i].Text = strings.Join(fields[:firstInput], " ")
		ast.Steps[i].Inputs = append([]string(nil), fields[firstInput:]...)
		if ast.Steps[i].Text == "" {
			return AST{}, fmt.Errorf("line %d: step has no description before its inputs", ast.Steps[i].Line)
		}
	}

	return ast, nil
}

// Transform resolves an AST into dependency trees and rejects cycles and
// branching results. An unnamed step with one input carries that input's name
// forward, so "melt BUTTER" followed by "mix BUTTER" remains a linear chain.
func Transform(ast AST) (Forest, error) {
	if len(ast.Steps) == 0 {
		return Forest{}, fmt.Errorf("AST has no steps")
	}

	nodes := make([]*Node, len(ast.Steps))
	currentResult := make(map[string]*Node)
	for i, step := range ast.Steps {
		if step.Text == "" {
			return Forest{}, fmt.Errorf("line %d: step has no description", step.Line)
		}
		nodes[i] = &Node{Text: step.Text, SourceLine: step.Line}
		if step.Result == "" {
			continue
		}
		if _, exists := currentResult[step.Result]; exists {
			return Forest{}, fmt.Errorf("line %d: result %q is declared more than once", step.Line, step.Result)
		}
		currentResult[step.Result] = nodes[i]
	}

	parents := make(map[*Node]*Node, len(nodes))
	for i, step := range ast.Steps {
		node := nodes[i]
		for _, input := range step.Inputs {
			child, exists := currentResult[input]
			if !exists {
				return Forest{}, fmt.Errorf("line %d: result %q is not declared", step.Line, input)
			}
			if parent := parents[child]; parent != nil {
				return Forest{}, fmt.Errorf(
					"line %d: result %q was already incorporated by line %d",
					step.Line,
					input,
					parent.SourceLine,
				)
			}
			parents[child] = node
			node.Children = append(node.Children, child)
		}
		if step.Result == "" && len(step.Inputs) == 1 {
			currentResult[step.Inputs[0]] = node
		}
	}

	state := make(map[*Node]uint8, len(nodes))
	var checkCycles func(*Node) error
	checkCycles = func(node *Node) error {
		switch state[node] {
		case 1:
			return fmt.Errorf("line %d: dependency cycle", node.SourceLine)
		case 2:
			return nil
		}
		state[node] = 1
		for _, child := range node.Children {
			if err := checkCycles(child); err != nil {
				return err
			}
		}
		state[node] = 2
		return nil
	}
	for _, node := range nodes {
		if err := checkCycles(node); err != nil {
			return Forest{}, err
		}
	}

	earliestSource := make(map[*Node]int, len(nodes))
	var findEarliestSource func(*Node) int
	findEarliestSource = func(node *Node) int {
		if source, exists := earliestSource[node]; exists {
			return source
		}
		source := node.SourceLine
		for _, child := range node.Children {
			source = min(source, findEarliestSource(child))
		}
		earliestSource[node] = source
		return source
	}
	for _, node := range nodes {
		findEarliestSource(node)
		sort.SliceStable(node.Children, func(i, j int) bool {
			return earliestSource[node.Children[i]] < earliestSource[node.Children[j]]
		})
	}

	var forest Forest
	for _, node := range nodes {
		if parents[node] == nil {
			forest.Roots = append(forest.Roots, node)
		}
	}
	sort.SliceStable(forest.Roots, func(i, j int) bool {
		return earliestSource[forest.Roots[i]] < earliestSource[forest.Roots[j]]
	})
	return forest, nil
}

type svgBox struct {
	node              *Node
	column            int
	firstRow, lastRow int
	root              bool
}

// layoutForest assigns one row to each leaf and places every incorporating
// step in the first column to the right of all of its inputs.
func layoutForest(forest Forest) ([]svgBox, int, int) {
	columns := make(map[*Node]int)
	var findColumn func(*Node) int
	findColumn = func(node *Node) int {
		if column, exists := columns[node]; exists {
			return column
		}
		column := 0
		for _, child := range node.Children {
			column = max(column, findColumn(child)+1)
		}
		columns[node] = column
		return column
	}

	maxColumn := 0
	for _, root := range forest.Roots {
		maxColumn = max(maxColumn, findColumn(root))
	}

	var boxes []svgBox
	row := 0
	var place func(*Node, bool)
	place = func(node *Node, root bool) {
		firstRow := row
		if len(node.Children) == 0 {
			row++
		} else {
			for _, child := range node.Children {
				place(child, false)
			}
		}
		boxes = append(boxes, svgBox{
			node:     node,
			column:   columns[node],
			firstRow: firstRow,
			lastRow:  row,
			root:     root,
		})
	}
	for _, root := range forest.Roots {
		place(root, true)
	}
	return boxes, maxColumn, row
}

func wrapWords(text string, limit int) []string {
	words := strings.Fields(text)
	lines := make([]string, 0, len(words))
	for _, word := range words {
		if len(lines) == 0 || utf8.RuneCountInString(lines[len(lines)-1])+1+utf8.RuneCountInString(word) > limit {
			lines = append(lines, word)
			continue
		}
		lines[len(lines)-1] += " " + word
	}
	return lines
}

// writeSVGText writes escaped, vertically centered SVG text.
func writeSVGText(out *strings.Builder, text string, x, centerY, lineHeight, wrapLimit int, anchor string) {
	lines := []string{text}
	if wrapLimit > 0 {
		lines = wrapWords(text, wrapLimit)
	}
	y := centerY - (len(lines)-1)*lineHeight/2
	fmt.Fprintf(out, `<text x="%d" y="%d" text-anchor="%s" dominant-baseline="middle">`, x, y, anchor)
	for i, line := range lines {
		if i == 0 {
			fmt.Fprintf(out, `<tspan x="%d">%s</tspan>`, x, html.EscapeString(line))
			continue
		}
		fmt.Fprintf(out, `<tspan x="%d" dy="%d">%s</tspan>`, x, lineHeight, html.EscapeString(line))
	}
	out.WriteString("</text>")
}

// Render returns a self-contained SVG table representing forest. It expects
// the dependency invariants established by Transform.
func Render(forest Forest) string {
	boxes, maxColumn, rows := layoutForest(forest)

	const (
		minLeafWidth     = 120
		minStepWidth     = 64
		textWidth        = 11
		horizontalSpace  = 16
		verticalSpace    = 8
		lineHeight       = 24
		minimumRowHeight = 36
	)
	columnWidths := make([]int, maxColumn)
	for i := range columnWidths {
		columnWidths[i] = minStepWidth
	}
	for _, box := range boxes {
		if len(box.node.Children) == 0 {
			continue
		}
		for _, word := range strings.Fields(box.node.Text) {
			column := box.column - 1
			columnWidths[column] = max(
				columnWidths[column],
				utf8.RuneCountInString(word)*textWidth+horizontalSpace,
			)
		}
	}

	stepsWidth := 0
	for _, columnWidth := range columnWidths {
		stepsWidth += columnWidth
	}
	minimumLeafWidth, maximumLeafWidth := minLeafWidth, minLeafWidth
	for _, box := range boxes {
		if len(box.node.Children) > 0 {
			continue
		}
		fullWidth := utf8.RuneCountInString(box.node.Text)*textWidth + horizontalSpace
		wordWidth := 0
		for _, word := range strings.Fields(box.node.Text) {
			wordWidth = max(wordWidth, utf8.RuneCountInString(word)*textWidth+horizontalSpace)
		}
		if box.root {
			fullWidth -= stepsWidth
			wordWidth -= stepsWidth
		}
		minimumLeafWidth = max(minimumLeafWidth, wordWidth)
		maximumLeafWidth = max(maximumLeafWidth, fullWidth)
	}

	internalBoxWidth := func(box svgBox) int {
		if !box.root {
			return columnWidths[box.column-1]
		}
		boxWidth := 0
		for _, columnWidth := range columnWidths[box.column-1:] {
			boxWidth += columnWidth
		}
		return boxWidth
	}
	measureRows := func(leafWidth int) ([]int, int) {
		width := leafWidth + stepsWidth
		rowHeights := make([]int, rows)
		for i := range rowHeights {
			rowHeights[i] = minimumRowHeight
		}
		for _, box := range boxes {
			if len(box.node.Children) > 0 {
				continue
			}
			boxWidth := leafWidth
			if box.root {
				boxWidth = width
			}
			wrapLimit := max(1, (boxWidth-horizontalSpace)/textWidth)
			rowHeights[box.firstRow] = max(
				rowHeights[box.firstRow],
				len(wrapWords(box.node.Text, wrapLimit))*lineHeight+verticalSpace,
			)
		}
		for _, box := range boxes {
			if len(box.node.Children) == 0 {
				continue
			}
			boxWidth := internalBoxWidth(box)
			wrapLimit := max(1, (boxWidth-horizontalSpace)/textWidth)
			requiredHeight := len(wrapWords(box.node.Text, wrapLimit))*lineHeight + verticalSpace
			currentHeight := 0
			for _, height := range rowHeights[box.firstRow:box.lastRow] {
				currentHeight += height
			}
			if currentHeight >= requiredHeight {
				continue
			}
			deficit := requiredHeight - currentHeight
			spannedRows := box.lastRow - box.firstRow
			for row := box.firstRow; row < box.lastRow; row++ {
				rowHeights[row] += deficit / spannedRows
				if row-box.firstRow < deficit%spannedRows {
					rowHeights[row]++
				}
			}
		}
		height := 0
		for _, rowHeight := range rowHeights {
			height += rowHeight
		}
		return rowHeights, height
	}

	leafWidth := minimumLeafWidth
	var rowHeights []int
	height := 0
	var bestArea int64 = -1
	// Widening the leaf column enlarges every row, while wrapping enlarges only
	// the affected rows. Minimum total area balances those two costs.
	for candidate := minimumLeafWidth; ; candidate = min(candidate+textWidth, maximumLeafWidth) {
		candidateRowHeights, candidateHeight := measureRows(candidate)
		area := int64(candidate+stepsWidth) * int64(candidateHeight)
		if bestArea < 0 || area < bestArea {
			leafWidth = candidate
			rowHeights = candidateRowHeights
			height = candidateHeight
			bestArea = area
		}
		if candidate == maximumLeafWidth {
			break
		}
	}

	width := leafWidth + stepsWidth
	columnEdges := make([]int, maxColumn+1)
	columnEdges[0] = leafWidth
	for i, columnWidth := range columnWidths {
		columnEdges[i+1] = columnEdges[i] + columnWidth
	}
	boxPosition := func(box svgBox) (int, int) {
		if len(box.node.Children) == 0 {
			if box.root {
				return 0, width
			}
			return 0, leafWidth
		}
		x := columnEdges[box.column-1]
		return x, internalBoxWidth(box)
	}

	rowEdges := make([]int, rows+1)
	for row, rowHeight := range rowHeights {
		rowEdges[row+1] = rowEdges[row] + rowHeight
	}

	var out strings.Builder
	fmt.Fprintf(&out, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d">`, width, height)
	fmt.Fprintf(&out, `<rect width="%d" height="%d" fill="#fff"/>`, width, height)
	out.WriteString(`<g fill="none" stroke="#159447" stroke-width="2" shape-rendering="crispEdges">`)
	fmt.Fprintf(&out, `<rect x="1" y="1" width="%d" height="%d"/>`, width-2, height-2)
	for _, box := range boxes {
		if !box.root || box.lastRow == rows {
			continue
		}
		fmt.Fprintf(
			&out,
			`<line x1="0" y1="%d" x2="%d" y2="%d"/>`,
			rowEdges[box.lastRow],
			width,
			rowEdges[box.lastRow],
		)
	}
	for _, box := range boxes {
		if box.root && len(box.node.Children) == 0 {
			continue
		}
		x, boxWidth := boxPosition(box)
		top, bottom := rowEdges[box.firstRow], rowEdges[box.lastRow]
		fmt.Fprintf(
			&out,
			`<rect x="%d" y="%d" width="%d" height="%d"/>`,
			x,
			top,
			boxWidth,
			bottom-top,
		)
		if len(box.node.Children) > 0 && !box.root {
			fmt.Fprintf(&out, `<line x1="0" y1="%d" x2="%d" y2="%d"/>`, bottom, x, bottom)
		}
	}
	out.WriteString("</g>")

	out.WriteString(`<g fill="#111" font-family="Arial, sans-serif" font-size="22">`)
	for _, box := range boxes {
		centerY := (rowEdges[box.firstRow] + rowEdges[box.lastRow]) / 2
		x, boxWidth := boxPosition(box)
		wrapLimit := max(1, (boxWidth-horizontalSpace)/textWidth)
		if len(box.node.Children) == 0 {
			if box.root {
				writeSVGText(&out, box.node.Text, width/2, centerY, lineHeight, wrapLimit, "middle")
			} else {
				writeSVGText(&out, box.node.Text, horizontalSpace/2, centerY, lineHeight, wrapLimit, "start")
			}
			continue
		}
		writeSVGText(&out, box.node.Text, x+boxWidth/2, centerY, lineHeight, wrapLimit, "middle")
	}
	out.WriteString("</g></svg>")
	return out.String()
}
