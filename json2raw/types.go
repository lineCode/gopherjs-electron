package main

import (
	"fmt"
	"io"
	"log"
	"reflect"
	"strings"
)

type Base struct {
	Name        string      `json:"name,omitempty"`
	RawType     interface{} `json:"type,omitempty"`
	Description string      `json:"description,omitempty"`

	Platforms []string `json:"platforms,omitempty"`
	Process   struct {
		Main     bool `json:"main,omitempty"`
		Renderer bool `json:"renderer,omitempty"`
	} `json:"process,omitempty"`

	Required bool `json:"required,omitempty"`

	Version    string `json:"version,omitempty"`
	RepoURL    string `json:"repoUrl,omitempty"`
	WebsiteURL string `json:"websiteUrl,omitempty"`
	Slug       string `json:"slug,omitempty"`
}

func (b *Base) Type() string {
	// default type set to string
	if b.RawType == nil {
		return ""
	}
	// v := reflect.Indirect(reflect.ValueOf(b.RawType))
	// if v.IsNil() {
	// 	return ""
	// }
	t := reflect.TypeOf(b.RawType)
	if t.Kind() == reflect.String {
		return b.RawType.(string)
	}
	if t.Kind() == reflect.Array {
		return b.RawType.([]string)[0]
	}
	return ""
}

var replacer = strings.NewReplacer(
	"`", " ",
	"\"", " ",
	".", " ",
	"_", " ",
	"-", " ",
	" ", " ",
	"\t", " ",
	"(", " ",
	")", " ",
	// name conversions
	"url", "URL",
	"Url", "URL",
	// ",", "",
)

func goSym(name string) string {
	name = replacer.Replace(name)
	name = strings.Title(name)
	name = strings.Replace(name, " ", "", -1)
	return name
}

func (b *Base) goSym() string {
	name := goSym(b.Name)
	if b.isModule() {
		return name + "Module"
	}
	return name
}

func (b *Base) comment(w io.Writer) {
	fmt.Fprintf(w, "\n// %s docs \n", b.goSym())
	fmt.Fprintf(w, "\n//%s", b.Description)
}

func (b *Base) isModule() bool {
	return b.Type() == "Module"
}

func (b *Base) isClass() bool {
	return b.Type() == "Class"
}

func (b *Base) isStructure() bool {
	return b.Type() == "Structure"
}

func (b *Base) isObject() bool {
	return b.Type() == "Object"
}

func (b *Base) isFunction() bool {
	return b.Type() == "Function"
}

func (b *Base) isBasic() bool {
	return !(b.isModule() ||
		b.isClass() ||
		b.isStructure() ||
		b.isFunction() ||
		b.isObject())
}

func basicType(typ string) string {
	switch typ {
	case "", "String":
		return "string"
	case "Integer", "INTEGER":
		return "int64"
	case "Number", "NUMBER", "Double", "DOUBLE", "Float", "FLOAT":
		return "float64"
	case "Boolean", "BOOLEAN":
		return "bool"
	}
	return "*js.Object"
}

func (b *Base) decl(w *Context, parent *Base) {
	fmt.Fprintf(w, "%s %s",
		b.goSym(),
		basicType(b.Type()),
	)
}

func (b *Base) annotate(w *Context, parent *Base) {
	if parent == nil || parent.isFunction() {
		return
	}
	fmt.Fprintf(w, " `js:\"%s\"` ", b.Name)
}

type Property struct {
	*Base
	Properties     []*Property      `json:"properties,omitempty"`     // object or structure
	Parameters     []*Property      `json:"parameters,omitempty"`     // func
	PossibleValues []*PossibleValue `json:"possibleValues,omitempty"` // const
}

func (p *Property) decl(w *Context, parent *Base) {
	if p.Name == "" {
		p.Name = "obj"
	}
	// possibleValues
	if p.PossibleValues != nil {
		tname := w.newConst(p, parent)
		fmt.Fprintf(w, "%s %s",
			p.goSym(),
			tname,
		)
		return
	}
	// basic types
	if p.isBasic() {
		p.Base.decl(w, parent)
		return
	}
	// compound types
	tname := w.newType(p, parent)
	fmt.Fprintf(w, "%s %s",
		p.goSym(),
		tname,
	)
}

type PossibleValue struct { // const
	*Base
	Value string `json:"value,omitempty"`
}

type Event struct {
	*Base
	Return []*Property `json:"returns,omitempty"`
}

// w      *io.Writer // main writer
// xw     *io.Writer // seperate writer of paramter/Property Objects
// parent *Base      // for Object
func (e Event) decl(w *Context, p *Base) {
	fmt.Fprintf(w,
		`Evt%s%s = "%s"`,
		strings.Replace(w.base.goSym(), "Module", "", 1),
		e.goSym(),
		e.Name,
	)
}

type Method struct {
	*Base
	Signature  string      `json:"signature,omitempty"`
	Parameters []*Property `json:"parameters,omitempty"`
	Return     *Property   `json:"returns,omitempty"`
}

func (m *Method) decl(w *Context, parent *Base) {
	fmt.Fprintf(w, "%s func(", m.goSym())
	// parameters
	for _, p := range m.Parameters {
		p.decl(w, m.Base)
		fmt.Fprintf(w, ",")
	}
	fmt.Fprintf(w, ")")
	// returns
	if m.Return != nil {
		fmt.Fprintf(w, "(")
		m.Return.decl(w, m.Base)
		fmt.Fprintf(w, ")")
	}
}

type Block struct {
	*Base
	// module
	Events     []*Event    `json:"events,omitempty"`
	Properties []*Property `json:"Properties,omitempty"`
	Methods    []*Method   `json:"Methods,omitempty"`
	// class
	InstanceName       string      `json:"instanceName,omitempty"`
	InstanceEvents     []*Event    `json:"instanceEvents,omitempty"`
	InstanceProperties []*Property `json:"instanceProperties,omitempty"`
	InstanceMethods    []*Method   `json:"instanceMethods,omitempty"`
	// standalone
	ConstructorMethod *Method   `json:"constructorMethod,omitempty"`
	StaticMethods     []*Method `json:"staticMethods,omitempty"`
}

type ApiFile []*Block

func (b *Block) declOther(w *Context) {
	// props
	fmt.Fprintf(w, "\ntype %s struct {\n\t", b.goSym())
	fmt.Fprintf(w, "*js.Object\n\t")
	declSlice(b.Properties, w, b.Base)
	fmt.Fprintf(w, "}\n")
}

func (b *Block) declModule(w *Context) {
	// evnents
	if len(b.Events) > 0 {
		fmt.Fprintf(w, "\nconst (\n\t")
		declSlice(b.Events, w, nil)
		fmt.Fprintf(w, ")\n")
	}
	// props and methods
	fmt.Fprintf(w, "\ntype %s struct {\n\t", b.goSym())
	fmt.Fprintf(w, "*js.Object\n\t")
	declSlice(b.Properties, w, b.Base)
	declSlice(b.Methods, w, b.Base)
	fmt.Fprintf(w, "}\n")
}

func (b *Block) declClass(w *Context) {
	// evnents
	if len(b.InstanceEvents) > 0 {
		fmt.Fprintf(w, "\nconst (\n\t")
		declSlice(b.InstanceEvents, w, nil)
		fmt.Fprintf(w, ")\n")
	}
	// props and methods
	fmt.Fprintf(w, "\ntype %s struct {\n\t", b.goSym())
	fmt.Fprintf(w, "*js.Object\n\t")
	declSlice(b.InstanceProperties, w, b.Base)
	declSlice(b.InstanceMethods, w, b.Base)
	fmt.Fprintf(w, "}\n")
	// static methods
}

func (a ApiFile) test() {
	for _, b := range a {
		if b.Name == "BrowserWindow" {
			log.Printf("BrowserWindow: %+v", *b)
		}
	}
}

func (a ApiFile) decl() {
	for _, b := range a {
		log.Println("Processing", b.Base.Name)
		ctx, err := newContext(b.Base)
		if err != nil {
			log.Println(b.Name, err)
		}
		if b.isModule() {
			b.declModule(ctx)
		} else if b.isClass() {
			b.declClass(ctx)
		} else {
			b.declOther(ctx)
		}
		ctx.declNewTypes()
		if err = ctx.Close(); err != nil {
			log.Println(b.Name, err)
		}
	}
	// a.test()
}