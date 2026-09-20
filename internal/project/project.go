package project

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rceman/agentir/internal/ir"
	"github.com/rceman/agentir/internal/source"
)

type Node struct {
	Span   ir.Span
	Kind   string
	Result string
}

// Projection contains the public AgentIR document plus exact patch metadata.
type Projection struct {
	Document ir.Document
	Nodes    map[string]Node
	Temps    map[string]ir.Span
}

// Project parses Go source and emits a deterministic AgentIR view.
func Project(path string, data []byte) (Projection, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, data, parser.ParseComments)
	if err != nil {
		return Projection{}, fmt.Errorf("parse %s: %w", path, err)
	}

	p := &projector{
		path:    filepath.ToSlash(filepath.Clean(path)),
		data:    data,
		fset:    fset,
		nodes:   make(map[string]Node),
		temps:   make(map[string]ir.Span),
		nodeIDs: make(map[ast.Node]string),
	}

	doc := ir.Document{
		Version:  ir.Version,
		Language: "go",
		Source: ir.SourceRef{
			Path:   p.path,
			SHA256: source.SHA256(data),
		},
	}

	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		fp := &functionProjector{
			projector: p,
			comments:  commentsInside(file.Comments, fn.Body),
		}
		fp.projectBlock(fn.Body, 0)
		doc.Functions = append(doc.Functions, ir.Function{
			Name:      functionName(fn),
			Signature: p.functionSignature(fn),
			Source:    p.span(fn.Pos(), fn.End()),
			Ops:       fp.ops,
		})
	}

	return Projection{Document: doc, Nodes: p.nodes, Temps: p.temps}, nil
}

type projector struct {
	path     string
	data     []byte
	fset     *token.FileSet
	nodes    map[string]Node
	temps    map[string]ir.Span
	nodeIDs  map[ast.Node]string
	nextNode int
}

func (p *projector) span(start, end token.Pos) ir.Span {
	s := p.fset.PositionFor(start, false)
	e := p.fset.PositionFor(end, false)
	return ir.Span{
		StartOffset: s.Offset,
		EndOffset:   e.Offset,
		StartLine:   s.Line,
		StartColumn: s.Column,
		EndLine:     e.Line,
		EndColumn:   e.Column,
	}
}

func (p *projector) nodeID(n ast.Node, span ir.Span, kind, result string) string {
	if id, ok := p.nodeIDs[n]; ok {
		return id
	}
	p.nextNode++
	id := "n" + strconv.Itoa(p.nextNode)
	p.nodeIDs[n] = id
	p.nodes[id] = Node{Span: span, Kind: kind, Result: result}
	if result != "" {
		p.temps[result] = span
	}
	return id
}

func (p *projector) render(n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, p.fset, n); err != nil {
		return "<unprintable>"
	}
	return b.String()
}

func (p *projector) functionSignature(fn *ast.FuncDecl) string {
	copyFn := *fn
	copyFn.Doc = nil
	copyFn.Body = nil
	var b bytes.Buffer
	if err := printer.Fprint(&b, p.fset, &copyFn); err != nil {
		return functionName(fn)
	}
	s := strings.TrimSpace(b.String())
	s = strings.TrimPrefix(s, "func ")
	return s
}

func functionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	return "(" + receiverName(fn.Recv.List[0].Type) + ")." + fn.Name.Name
}

func receiverName(expr ast.Expr) string {
	switch x := expr.(type) {
	case *ast.Ident:
		return x.Name
	case *ast.StarExpr:
		return "*" + receiverName(x.X)
	case *ast.IndexExpr:
		return receiverName(x.X)
	case *ast.IndexListExpr:
		return receiverName(x.X)
	default:
		return "recv"
	}
}

func commentsInside(all []*ast.CommentGroup, body *ast.BlockStmt) []*ast.CommentGroup {
	var out []*ast.CommentGroup
	for _, group := range all {
		if group.Pos() > body.Lbrace && group.End() < body.Rbrace {
			out = append(out, group)
		}
	}
	return out
}

type functionProjector struct {
	projector   *projector
	ops         []ir.Op
	temp        int
	comments    []*ast.CommentGroup
	commentNext int
}

func (f *functionProjector) projectBlock(block *ast.BlockStmt, depth int) {
	for _, stmt := range block.List {
		f.emitCommentsBefore(stmt.Pos(), depth)
		f.projectStmt(stmt, depth)
	}
	f.emitCommentsBefore(block.Rbrace, depth)
}

func (f *functionProjector) emitCommentsBefore(limit token.Pos, depth int) {
	for f.commentNext < len(f.comments) && f.comments[f.commentNext].Pos() < limit {
		group := f.comments[f.commentNext]
		span := f.projector.span(group.Pos(), group.End())
		text := strings.TrimSpace(group.Text())
		if text == "" {
			text = "//"
		} else {
			lines := strings.Split(text, "
")
			for i := range lines {
				lines[i] = "// " + strings.TrimSpace(lines[i])
			}
			text = strings.Join(lines, " | ")
		}
		f.ops = append(f.ops, ir.Op{Kind: "comment", Text: text, Depth: depth, Source: span})
		f.commentNext++
	}
}

func (f *functionProjector) projectStmt(stmt ast.Stmt, depth int) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		values := make([]string, 0, len(s.Rhs))
		for _, rhs := range s.Rhs {
			values = append(values, f.projectExpr(rhs, depth, false))
		}
		lhs := make([]string, 0, len(s.Lhs))
		for _, item := range s.Lhs {
			lhs = append(lhs, f.projector.render(item))
		}
		f.emitPatchable(s, "assign", "", strings.Join(lhs, ", ")+" "+s.Tok.String()+" "+strings.Join(values, ", "), depth)

	case *ast.DeclStmt:
		f.emitPatchable(s, "decl", "", f.projector.render(s), depth)

	case *ast.ExprStmt:
		value := f.projectExpr(s.X, depth, false)
		f.emitPatchable(s, "expr", "", value, depth)

	case *ast.ReturnStmt:
		values := make([]string, 0, len(s.Results))
		for _, result := range s.Results {
			values = append(values, f.projectExpr(result, depth, false))
		}
		text := "return"
		if len(values) > 0 {
			text += " " + strings.Join(values, ", ")
		}
		f.emitPatchable(s, "return", "", text, depth)

	case *ast.IfStmt:
		if s.Init != nil {
			f.projectStmt(s.Init, depth)
		}
		condition := f.projectExpr(s.Cond, depth, false)
		f.emitStructural(s, "if", "if "+condition, depth)
		f.projectBlock(s.Body, depth+1)
		if s.Else != nil {
			f.emitStructural(s.Else, "else", "else", depth)
			switch e := s.Else.(type) {
			case *ast.BlockStmt:
				f.projectBlock(e, depth+1)
			case *ast.IfStmt:
				f.projectStmt(e, depth+1)
			default:
				f.projectStmt(e, depth+1)
			}
		}

	case *ast.ForStmt:
		if s.Init != nil {
			f.projectStmt(s.Init, depth)
		}
		condition := "true"
		if s.Cond != nil {
			condition = f.projectExpr(s.Cond, depth, false)
		}
		f.emitStructural(s, "for", "for "+condition, depth)
		f.projectBlock(s.Body, depth+1)
		if s.Post != nil {
			f.projectStmt(s.Post, depth+1)
		}

	case *ast.RangeStmt:
		rangeValue := f.projectExpr(s.X, depth, false)
		left := ""
		if s.Key != nil {
			left = f.projector.render(s.Key)
			if s.Value != nil {
				left += ", " + f.projector.render(s.Value)
			}
			left += " " + s.Tok.String() + " "
		}
		f.emitStructural(s, "range", "for "+left+"range "+rangeValue, depth)
		f.projectBlock(s.Body, depth+1)

	case *ast.SwitchStmt:
		if s.Init != nil {
			f.projectStmt(s.Init, depth)
		}
		tag := ""
		if s.Tag != nil {
			tag = " " + f.projectExpr(s.Tag, depth, false)
		}
		f.emitStructural(s, "switch", "switch"+tag, depth)
		for _, item := range s.Body.List {
			clause, ok := item.(*ast.CaseClause)
			if !ok {
				continue
			}
			f.projectCaseClause(clause, depth+1)
		}

	case *ast.TypeSwitchStmt:
		if s.Init != nil {
			f.projectStmt(s.Init, depth)
		}
		assign := f.projector.render(s.Assign)
		f.emitStructural(s, "type_switch", "switch "+assign, depth)
		for _, item := range s.Body.List {
			clause, ok := item.(*ast.CaseClause)
			if !ok {
				continue
			}
			f.projectCaseClause(clause, depth+1)
		}

	case *ast.SelectStmt:
		f.emitStructural(s, "select", "select", depth)
		for _, item := range s.Body.List {
			clause, ok := item.(*ast.CommClause)
			if !ok {
				continue
			}
			label := "default"
			if clause.Comm != nil {
				label = "case " + f.projector.render(clause.Comm)
			}
			f.emitStructural(clause, "comm_case", label, depth+1)
			for _, bodyStmt := range clause.Body {
				f.projectStmt(bodyStmt, depth+2)
			}
		}

	case *ast.LabeledStmt:
		f.emitStructural(s, "label", s.Label.Name+":", depth)
		f.projectStmt(s.Stmt, depth+1)

	case *ast.BranchStmt:
		f.emitPatchable(s, "branch", "", f.projector.render(s), depth)

	case *ast.IncDecStmt:
		f.emitPatchable(s, "incdec", "", f.projector.render(s), depth)

	case *ast.SendStmt:
		f.emitPatchable(s, "send", "", f.projector.render(s), depth)

	case *ast.GoStmt:
		value := f.projectExpr(s.Call, depth, false)
		f.emitPatchable(s, "go", "", "go "+value, depth)

	case *ast.DeferStmt:
		value := f.projectExpr(s.Call, depth, false)
		f.emitPatchable(s, "defer", "", "defer "+value, depth)

	case *ast.BlockStmt:
		f.projectBlock(s, depth)

	case *ast.EmptyStmt:
		return

	default:
		// Unsupported compound syntax remains visible but is not directly patchable.
		f.emitStructural(stmt, "opaque", "opaque "+f.projector.render(stmt), depth)
	}
}

func (f *functionProjector) projectCaseClause(clause *ast.CaseClause, depth int) {
	label := "default"
	if len(clause.List) > 0 {
		items := make([]string, 0, len(clause.List))
		for _, expr := range clause.List {
			items = append(items, f.projectExpr(expr, depth, false))
		}
		label = "case " + strings.Join(items, ", ")
	}
	f.emitStructural(clause, "case", label, depth)
	for _, stmt := range clause.Body {
		f.projectStmt(stmt, depth+1)
	}
}

func (f *functionProjector) projectExpr(expr ast.Expr, depth int, materialize bool) string {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return f.projectExpr(e.X, depth, materialize)

	case *ast.CallExpr:
		fun := f.projectExprReference(e.Fun)
		args := make([]string, 0, len(e.Args))
		for _, arg := range e.Args {
			args = append(args, f.projectExpr(arg, depth, true))
		}
		text := fun + "(" + strings.Join(args, ", ")
		if e.Ellipsis.IsValid() && len(args) > 0 {
			text += "..."
		}
		text += ")"
		if !materialize {
			return text
		}
		result := f.nextTemp()
		f.emitExpr(e, "call", result, text, depth)
		return result

	case *ast.BinaryExpr:
		left := f.projectExpr(e.X, depth, true)
		right := f.projectExpr(e.Y, depth, true)
		text := left + " " + e.Op.String() + " " + right
		if !materialize {
			return text
		}
		result := f.nextTemp()
		f.emitExpr(e, "binary", result, text, depth)
		return result

	case *ast.UnaryExpr:
		value := f.projectExpr(e.X, depth, true)
		text := e.Op.String() + value
		if !materialize {
			return text
		}
		result := f.nextTemp()
		f.emitExpr(e, "unary", result, text, depth)
		return result

	case *ast.IndexExpr:
		target := f.projectExpr(e.X, depth, true)
		index := f.projectExpr(e.Index, depth, true)
		text := target + "[" + index + "]"
		if !materialize {
			return text
		}
		result := f.nextTemp()
		f.emitExpr(e, "index", result, text, depth)
		return result

	case *ast.SliceExpr:
		target := f.projectExpr(e.X, depth, true)
		low, high, max := "", "", ""
		if e.Low != nil {
			low = f.projectExpr(e.Low, depth, true)
		}
		if e.High != nil {
			high = f.projectExpr(e.High, depth, true)
		}
		if e.Max != nil {
			max = f.projectExpr(e.Max, depth, true)
		}
		inside := low + ":" + high
		if e.Slice3 {
			inside += ":" + max
		}
		text := target + "[" + inside + "]"
		if !materialize {
			return text
		}
		result := f.nextTemp()
		f.emitExpr(e, "slice", result, text, depth)
		return result

	case *ast.TypeAssertExpr:
		target := f.projectExpr(e.X, depth, true)
		typeText := "type"
		if e.Type != nil {
			typeText = f.projector.render(e.Type)
		}
		text := target + ".(" + typeText + ")"
		if !materialize {
			return text
		}
		result := f.nextTemp()
		f.emitExpr(e, "type_assert", result, text, depth)
		return result

	case *ast.StarExpr:
		value := f.projectExpr(e.X, depth, true)
		text := "*" + value
		if !materialize {
			return text
		}
		result := f.nextTemp()
		f.emitExpr(e, "deref", result, text, depth)
		return result

	case *ast.SelectorExpr:
		receiver := f.projectExpr(e.X, depth, true)
		return receiver + "." + e.Sel.Name

	case *ast.Ident:
		return e.Name

	case *ast.BasicLit:
		return e.Value

	case *ast.IndexListExpr:
		return f.projector.render(expr)

	case *ast.FuncLit, *ast.CompositeLit, *ast.KeyValueExpr, *ast.ArrayType, *ast.MapType,
		*ast.ChanType, *ast.InterfaceType, *ast.StructType, *ast.FuncType:
		return f.projector.render(expr)

	default:
		return f.projector.render(expr)
	}
}

func (f *functionProjector) projectExprReference(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return f.projectExprReference(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr, *ast.IndexListExpr:
		return f.projector.render(expr)
	default:
		return f.projectExpr(expr, 0, true)
	}
}

func (f *functionProjector) nextTemp() string {
	f.temp++
	return "$" + strconv.Itoa(f.temp)
}

func (f *functionProjector) emitExpr(expr ast.Expr, kind, result, text string, depth int) {
	span := f.projector.span(expr.Pos(), expr.End())
	id := f.projector.nodeID(expr, span, kind, result)
	f.ops = append(f.ops, ir.Op{ID: id, Kind: kind, Result: result, Text: text, Depth: depth, Source: span})
}

func (f *functionProjector) emitPatchable(node ast.Node, kind, result, text string, depth int) {
	span := f.projector.span(node.Pos(), node.End())
	id := f.projector.nodeID(node, span, kind, result)
	f.ops = append(f.ops, ir.Op{ID: id, Kind: kind, Result: result, Text: text, Depth: depth, Source: span})
}

func (f *functionProjector) emitStructural(node ast.Node, kind, text string, depth int) {
	span := f.projector.span(node.Pos(), node.End())
	f.ops = append(f.ops, ir.Op{Kind: kind, Text: text, Depth: depth, Source: span})
}
