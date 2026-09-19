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

// Projection contains the public document plus exact projected-node spans used by the patcher.
type Projection struct {
	Document ir.Document
	Nodes    map[string]ir.Span
}

// Project parses Go source and emits a deterministic, linearized AgentIR view.
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
		nodes:   make(map[string]ir.Span),
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
		fp := &functionProjector{projector: p}
		fp.projectBlock(fn.Body, 0)
		doc.Functions = append(doc.Functions, ir.Function{
			Name:   functionName(fn),
			Source: p.span(fn.Pos(), fn.End()),
			Ops:    fp.ops,
		})
	}

	return Projection{Document: doc, Nodes: p.nodes}, nil
}

type projector struct {
	path     string
	data     []byte
	fset     *token.FileSet
	nodes    map[string]ir.Span
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

func (p *projector) nodeID(n ast.Node, span ir.Span) string {
	if id, ok := p.nodeIDs[n]; ok {
		return id
	}
	p.nextNode++
	id := "n" + strconv.Itoa(p.nextNode)
	p.nodeIDs[n] = id
	p.nodes[id] = span
	return id
}

func (p *projector) render(n ast.Node) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, p.fset, n); err != nil {
		return "<unprintable>"
	}
	return b.String()
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

type functionProjector struct {
	projector *projector
	ops       []ir.Op
	temp      int
}

func (f *functionProjector) projectBlock(block *ast.BlockStmt, depth int) {
	for _, stmt := range block.List {
		f.projectStmt(stmt, depth)
	}
}

func (f *functionProjector) projectStmt(stmt ast.Stmt, depth int) {
	switch s := stmt.(type) {
	case *ast.AssignStmt:
		values := make([]string, 0, len(s.Rhs))
		for _, rhs := range s.Rhs {
			values = append(values, f.projectExpr(rhs, depth))
		}
		lhs := make([]string, 0, len(s.Lhs))
		for _, item := range s.Lhs {
			lhs = append(lhs, f.projector.render(item))
		}
		f.emitNode(s, "assign", "", strings.Join(lhs, ", ")+" "+s.Tok.String()+" "+strings.Join(values, ", "), depth)

	case *ast.DeclStmt:
		f.emitNode(s, "decl", "", f.projector.render(s), depth)

	case *ast.ExprStmt:
		value := f.projectExpr(s.X, depth)
		if _, ok := s.X.(*ast.CallExpr); !ok {
			f.emitNode(s, "expr", "", value, depth)
		}

	case *ast.ReturnStmt:
		values := make([]string, 0, len(s.Results))
		for _, result := range s.Results {
			values = append(values, f.projectExpr(result, depth))
		}
		text := "return"
		if len(values) > 0 {
			text += " " + strings.Join(values, ", ")
		}
		f.emitNode(s, "return", "", text, depth)

	case *ast.IfStmt:
		if s.Init != nil {
			f.projectStmt(s.Init, depth)
		}
		condition := f.projectExpr(s.Cond, depth)
		f.emitNode(s, "if", "", "if "+condition, depth)
		f.projectBlock(s.Body, depth+1)
		if s.Else != nil {
			f.emitNode(s.Else, "else", "", "else", depth)
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
			condition = f.projectExpr(s.Cond, depth)
		}
		f.emitNode(s, "for", "", "for "+condition, depth)
		f.projectBlock(s.Body, depth+1)
		if s.Post != nil {
			f.projectStmt(s.Post, depth+1)
		}

	case *ast.RangeStmt:
		rangeValue := f.projectExpr(s.X, depth)
		left := ""
		if s.Key != nil {
			left = f.projector.render(s.Key)
			if s.Value != nil {
				left += ", " + f.projector.render(s.Value)
			}
			left += " " + s.Tok.String() + " "
		}
		f.emitNode(s, "range", "", "for "+left+"range "+rangeValue, depth)
		f.projectBlock(s.Body, depth+1)

	case *ast.BranchStmt:
		f.emitNode(s, "branch", "", f.projector.render(s), depth)

	case *ast.IncDecStmt:
		f.emitNode(s, "incdec", "", f.projector.render(s), depth)

	case *ast.GoStmt:
		value := f.projectExpr(s.Call, depth)
		f.emitNode(s, "go", "", "go "+value, depth)

	case *ast.DeferStmt:
		value := f.projectExpr(s.Call, depth)
		f.emitNode(s, "defer", "", "defer "+value, depth)

	case *ast.BlockStmt:
		f.projectBlock(s, depth)

	default:
		f.emitNode(stmt, "stmt", "", f.projector.render(stmt), depth)
	}
}

func (f *functionProjector) projectExpr(expr ast.Expr, depth int) string {
	switch e := expr.(type) {
	case *ast.ParenExpr:
		return f.projectExpr(e.X, depth)

	case *ast.CallExpr:
		fun := f.projectExprReference(e.Fun)
		args := make([]string, 0, len(e.Args))
		for _, arg := range e.Args {
			args = append(args, f.projectExpr(arg, depth))
		}
		result := f.nextTemp()
		f.emitExpr(e, "call", result, "call "+fun+"("+strings.Join(args, ", ")+")", depth)
		return result

	case *ast.BinaryExpr:
		left := f.projectExpr(e.X, depth)
		right := f.projectExpr(e.Y, depth)
		result := f.nextTemp()
		f.emitExpr(e, "binary", result, left+" "+e.Op.String()+" "+right, depth)
		return result

	case *ast.UnaryExpr:
		value := f.projectExpr(e.X, depth)
		result := f.nextTemp()
		f.emitExpr(e, "unary", result, e.Op.String()+value, depth)
		return result

	case *ast.IndexExpr:
		target := f.projectExpr(e.X, depth)
		index := f.projectExpr(e.Index, depth)
		result := f.nextTemp()
		f.emitExpr(e, "index", result, target+"["+index+"]", depth)
		return result

	case *ast.SliceExpr:
		target := f.projectExpr(e.X, depth)
		low := ""
		high := ""
		max := ""
		if e.Low != nil {
			low = f.projectExpr(e.Low, depth)
		}
		if e.High != nil {
			high = f.projectExpr(e.High, depth)
		}
		if e.Max != nil {
			max = f.projectExpr(e.Max, depth)
		}
		inside := low + ":" + high
		if e.Slice3 {
			inside += ":" + max
		}
		result := f.nextTemp()
		f.emitExpr(e, "slice", result, target+"["+inside+"]", depth)
		return result

	case *ast.TypeAssertExpr:
		target := f.projectExpr(e.X, depth)
		result := f.nextTemp()
		typeText := "type"
		if e.Type != nil {
			typeText = f.projector.render(e.Type)
		}
		f.emitExpr(e, "type_assert", result, target+".("+typeText+")", depth)
		return result

	case *ast.StarExpr:
		value := f.projectExpr(e.X, depth)
		result := f.nextTemp()
		f.emitExpr(e, "deref", result, "*"+value, depth)
		return result

	case *ast.SelectorExpr:
		// Keep selectors compact. Their receiver may itself contain work that should be expanded.
		receiver := f.projectExpr(e.X, depth)
		return receiver + "." + e.Sel.Name

	case *ast.Ident:
		return e.Name

	case *ast.BasicLit:
		return e.Value

	case *ast.FuncLit, *ast.CompositeLit, *ast.KeyValueExpr, *ast.ArrayType, *ast.MapType,
		*ast.ChanType, *ast.InterfaceType, *ast.StructType, *ast.FuncType, *ast.IndexListExpr:
		return f.projector.render(expr)

	default:
		return f.projector.render(expr)
	}
}

func (f *functionProjector) projectExprReference(expr ast.Expr) string {
	// Function values should stay recognisable. Only expand genuinely nested computations.
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return f.projectExprReference(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr, *ast.IndexListExpr:
		return f.projector.render(expr)
	default:
		return f.projectExpr(expr, 0)
	}
}

func (f *functionProjector) nextTemp() string {
	f.temp++
	return "$t" + strconv.Itoa(f.temp)
}

func (f *functionProjector) emitExpr(expr ast.Expr, kind, result, text string, depth int) {
	span := f.projector.span(expr.Pos(), expr.End())
	f.ops = append(f.ops, ir.Op{
		ID:     f.projector.nodeID(expr, span),
		Kind:   kind,
		Result: result,
		Text:   text,
		Depth:  depth,
		Source: span,
	})
}

func (f *functionProjector) emitNode(node ast.Node, kind, result, text string, depth int) {
	span := f.projector.span(node.Pos(), node.End())
	f.ops = append(f.ops, ir.Op{
		ID:     f.projector.nodeID(node, span),
		Kind:   kind,
		Result: result,
		Text:   text,
		Depth:  depth,
		Source: span,
	})
}
