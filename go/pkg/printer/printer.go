// This file implements printing of AST nodes.

package printer

import (
	"fmt"
	"github.com/hknutzen/Netspoc/go/pkg/ast"
	"strings"
)

type printer struct {
	src []byte // Original source code with comments.
	// Current state
	output []byte // raw printer result
	indent int    // current indentation
}

func (p *printer) init(src []byte) {
	p.src = src
}

func (p *printer) print(line string) {
	if line != "" {
		for i := 0; i < p.indent; i++ {
			p.output = append(p.output, ' ')
		}
		p.output = append(p.output, []byte(line)...)
	}
	p.output = append(p.output, '\n')
}

func (p *printer) emptyLine() {
	l := len(p.output)
	if l < 2 || p.output[l-1] != '\n' || p.output[l-2] != '\n' {
		p.output = append(p.output, '\n')
	}
}

func utfLen(s string) int {
	return len([]rune(s))
}

func isShort(l []ast.Element) string {
	if len(l) == 1 {
		switch x := l[0].(type) {
		case *ast.NamedRef:
			return x.Type + ":" + x.Name
		case *ast.User:
			return "user"
		}
	}
	return ""
}

func (p *printer) subElements(p1, p2 string, l []ast.Element, stop string) {
	if name := isShort(l); name != "" {
		if !strings.HasSuffix(p2, "[") {
			p2 += " "
		}
		p.print(p1 + p2 + name + stop)
	} else {
		p.print(p1 + p2)
		ind := utfLen(p1)
		p.indent += ind
		p.elementList(l, stop)
		p.indent -= ind
	}
}

func (p *printer) element(pre string, el ast.Element, post string) {
	switch x := el.(type) {
	case *ast.NamedRef:
		p.print(pre + x.Type + ":" + x.Name + post)
	case *ast.IntfRef:
		ext := x.Extension
		net := x.Network
		if net == "[" {
			net = "[" + ext + "]"
			ext = ""
		} else if ext != "" {
			ext = "." + ext
		}
		p.print(pre + x.Type + ":" + x.Router + "." + net + ext + post)
	case *ast.SimpleAuto:
		p.subElements(pre, x.Type+":[", x.Elements, "]"+post)
	case *ast.AggAuto:
		p2 := x.Type + ":["
		if n := x.Net; n != nil {
			p2 += "ip = " + n.String() + " &"
		}
		p.subElements(pre, p2, x.Elements, "]"+post)
	case *ast.IntfAuto:
		p2 := x.Type + ":["
		stop := "].[" + x.Selector + "]" + post
		if x.Managed {
			p2 += "managed &"
		}
		p.subElements(pre, p2, x.Elements, stop)
	case *ast.Intersection:
		p.intersection(pre, x.Elements, post)
	case *ast.Complement:
		p.element("! ", x.Element, post)
	case *ast.User:
		p.print(pre + "user" + post)
	default:
		panic(fmt.Sprintf("Unknown element: %T", el))
	}
}

func (p *printer) intersection(pre string, l []ast.Element, post string) {
	// First element already gets pre comment from union.
	p.element(pre, l[0], p.TrailingComment(l[0], "&!"))
	ind := utfLen(pre)
	p.indent += ind
	for _, el := range l[1:] {
		pre := "&"
		if x, ok := el.(*ast.Complement); ok {
			pre += "!"
			el = x.Element
		}
		pre += " "
		p.PreComment(el, "&!")
		p.element(pre, el, p.TrailingComment(el, "&!,;"))
	}
	p.print(post)
	p.indent -= ind
}

func (p *printer) elementList(l []ast.Element, stop string) {
	p.indent++
	for _, el := range l {
		p.PreComment(el, ",")
		post := ","
		if _, ok := el.(*ast.Intersection); ok {
			// Intersection already prints comments of its elements.
			p.element("", el, post)
		} else {
			p.element("", el, post+p.TrailingComment(el, ",;"))
		}
	}
	p.indent--
	p.print(stop)
}

func (p *printer) topElementList(l []ast.Element) {
	p.elementList(l, ";")
}

func (p *printer) topProtocol(n *ast.Protocol) {
	p.indent++
	p.print(n.Value + ";" + p.TrailingComment(n, ";"))
	p.indent--
}

func (p *printer) topProtocolList(l []*ast.Value) {
	p.indent++
	for _, el := range l {
		p.PreComment(el, ",")
		p.print(el.Value + "," + p.TrailingComment(el, ",;"))
	}
	p.indent--
	p.print(";")
}

func (p *printer) namedList(name string, l []ast.Element) {
	pre := name + " = "
	if len(l) == 0 {
		p.print(pre + ";")
		return
	}

	// Put first value on same line with name, if it has no comment.
	first := l[0]
	var rest []ast.Element
	ind := utfLen(pre)
	if p.hasPreComment(first, ",") {
		p.print(pre[:ind-1])
		rest = l
	} else {
		rest = l[1:]
		var post string
		if len(rest) == 0 {
			post = ";"
		} else {
			post = ","
		}
		p.element(pre, first, post+p.TrailingComment(first, ",;"))
	}

	// Show other lines with same indentation as first line.
	if len(rest) != 0 {
		p.indent += ind
		for _, v := range rest {
			p.PreComment(v, ",")
			p.element("", v, ","+p.TrailingComment(v, ",;"))
		}
		p.print(";")
		p.indent -= ind
	}
}

func (p *printer) namedUnion(pre string, n *ast.NamedUnion) {
	p.PreComment(n, "")
	p.namedList(pre+n.Name, n.Elements)
}

const shortName = 10

func (p *printer) namedValueList(name string, l []*ast.Value) {

	// Put first value(s) on same line with name, if it has no comment.
	first := l[0]
	var rest []*ast.Value
	pre := name + " = "
	var ind int
	if p.hasPreComment(first, ",") ||
		(len(name) > shortName && len(l) > 1) {

		p.print(pre[:len(pre)-1])
		ind = 1
		rest = l
	} else if name == "model" || len(l) == 1 {
		line := getValueList(l)
		p.print(pre + line + p.TrailingComment(l[len(l)-1], ",;"))
	} else {
		ind = utfLen(pre)
		rest = l[1:]
		var post string
		if len(rest) == 0 {
			post = ";"
		} else {
			post = ","
		}
		p.print(pre + first.Value + post + p.TrailingComment(first, ",;"))
	}

	// Show other lines with same indentation as first line.
	if len(rest) != 0 {
		p.indent += ind
		for _, v := range rest {
			p.PreComment(v, ",")
			p.print(v.Value + "," + p.TrailingComment(v, ",;"))
		}
		if ind == 1 {
			p.indent -= ind
			p.print(";")
		} else {
			p.print(";")
			p.indent -= ind
		}
	}
}

func (p *printer) complexValue(name string, l []*ast.Attribute) {
	pre := name + " = {"
	p.print(pre)
	p.indent++
	for _, a := range l {
		p.attribute(a)
	}
	p.indent--
	p.print("}")
}

func (p *printer) attribute(n *ast.Attribute) {
	p.PreComment(n, "")
	if l := n.ValueList; l != nil {
		p.namedValueList(n.Name, l)
	} else if l := n.ComplexValue; l != nil {
		name := n.Name
		if name == "virtual" || strings.Index(name, ":") != -1 {
			p.print(name + " = {" + getAttrList(l) + " }")
		} else {
			p.complexValue(name, l)
		}
	} else {
		// Short attribute without values.
		p.print(n.Name + ";" + p.TrailingComment(n, ",;"))
	}
}

func (p *printer) attributeList(l []*ast.Attribute) {
	p.indent++
	for _, a := range l {
		p.attribute(a)
	}
	p.indent--
}

func (p *printer) rule(n *ast.Rule) {
	p.PreComment(n, "")
	action := "permit"
	if n.Deny {
		action = "deny  "
	}
	action += " "
	ind := len(action)
	p.namedUnion(action, n.Src)
	p.indent += ind
	p.namedUnion("", n.Dst)
	p.attribute(n.Prt)
	if a := n.Log; a != nil {
		p.attribute(a)
	}
	p.indent -= ind
}

func (p *printer) service(n *ast.Service) {
	p.emptyLine()
	p.attributeList(n.Attributes)
	p.emptyLine()
	p.indent++
	if n.Foreach {
		p.print("user = foreach")
		p.elementList(n.User.Elements, ";")
	} else {
		p.namedUnion("", n.User)
	}
	for _, r := range n.Rules {
		p.rule(r)
	}
	p.indent--
	p.print("}")
}

func getValueList(l []*ast.Value) string {
	line := ""
	for _, v := range l {
		if line != "" {
			line += ", "
		}
		line += v.Value
	}
	return line + ";"
}

func getAttr(n *ast.Attribute) string {
	if l := n.ValueList; l != nil {
		return n.Name + " = " + getValueList(l)
	}
	if l := n.ComplexValue; l != nil {
		return n.Name + " = {" + getAttrList(l) + " }"
	} else {
		return n.Name + ";"
	}
}

func getAttrList(l []*ast.Attribute) string {
	var line string
	for _, a := range l {
		line += " " + getAttr(a)
	}
	return line
}

func (p *printer) indentedAttribute(n *ast.Attribute, max int) {
	p.PreComment(n, "")
	if l := n.ComplexValue; l != nil {
		name := n.Name
		if len := utfLen(name); len < max {
			name += strings.Repeat(" ", max-len)
		}
		p.print(name + " = {" + getAttrList(l) + " }" +
			p.TrailingComment(n, "}"))
	} else {
		// Short attribute without values.
		p.print(n.Name + ";" + p.TrailingComment(n, ",;"))
	}
}

func getMaxAndNoIndent(
	l []*ast.Attribute, simple map[string]bool) (int, map[*ast.Attribute]bool) {

	max := 0
	noIndent := make(map[*ast.Attribute]bool)
ATTR:
	for _, a := range l {
		if l2 := a.ComplexValue; l2 != nil {
			for _, a2 := range l2 {
				if !simple[a2.Name] {
					noIndent[a] = true
					continue ATTR
				}
			}
			if len := utfLen(a.Name); len > max {
				max = len
			}
		}
	}
	return max, noIndent
}

func (p *printer) indentedAttributeList(
	l []*ast.Attribute, simple map[string]bool) {

	p.indent++
	max, noIndent := getMaxAndNoIndent(l, simple)
	for _, a := range l {
		if noIndent[a] {
			p.complexValue(a.Name, a.ComplexValue)
		} else {
			p.indentedAttribute(a, max)
		}
	}
	p.indent--
}

var simpleHostAttr = map[string]bool{
	"ip":    true,
	"range": true,
	"owner": true,
}

func (p *printer) network(n *ast.Network) {
	p.attributeList(n.Attributes)
	p.indentedAttributeList(n.Hosts, simpleHostAttr)
	p.print("}")
}

var simpleIntfAttr = map[string]bool{
	"ip":         true,
	"unnumbered": true,
	"negotiated": true,
	"hardware":   true,
	"loopback":   true,
	"vip":        true,
	"owner":      true,
}

func (p *printer) router(n *ast.Router) {
	p.attributeList(n.Attributes)
	p.indentedAttributeList(n.Interfaces, simpleIntfAttr)
	p.print("}")
}

func (p *printer) topStruct(n *ast.TopStruct) {
	p.attributeList(n.Attributes)
	p.print("}")
}

func (p *printer) toplevel(n ast.Toplevel) {
	p.PreComment(n, "")
	sep := " ="
	trailing := ""
	d := n.GetDescription()
	if n.IsStruct() {
		sep += " {"
	}
	if n.IsStruct() || d != nil {
		pos := n.Pos() + len(n.GetName())
		// Don't print trailing comment for list without description. It
		// will be printed as PreComment of first element.
		trailing = p.TrailingCommentAt(pos, sep)
	}
	p.print(n.GetName() + sep + trailing)

	if d != nil {
		p.indent++
		p.PreComment(d, sep)
		p.print("description =" + d.Text + p.TrailingComment(d, "="))
		p.indent--
		p.emptyLine()
	}

	switch x := n.(type) {
	case *ast.TopStruct:
		p.topStruct(x)
	case *ast.TopList:
		p.topElementList(x.Elements)
	case *ast.Protocol:
		p.topProtocol(x)
	case *ast.Protocolgroup:
		p.topProtocolList(x.ValueList)
	case *ast.Service:
		p.service(x)
	case *ast.Network:
		p.network(x)
	case *ast.Router:
		p.router(x)
	default:
		panic(fmt.Sprintf("Unknown type: %T", n))
	}
}

func File(list []ast.Toplevel, src []byte) []byte {
	p := new(printer)
	p.init(src)

	for i, t := range list {
		p.toplevel(t)
		// Add empty line between output.
		if i != len(list)-1 {
			p.print("")
		}
	}

	return p.output
}
