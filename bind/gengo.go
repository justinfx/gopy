// Copyright 2015 The go-python Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package bind

import (
	"fmt"
	"go/token"
	"go/types"
	"path/filepath"
	"strings"
)

const (
	goPreamble = `// Package main is an autogenerated binder stub for package %[1]s.
// gopy gen -lang=go %[1]s
//
// File is generated by gopy gen. Do not edit.
package main

//#cgo pkg-config: python2 --cflags --libs
//#include <stdlib.h>
//#include <string.h>
import "C"

import (
	"unsafe"

	%[2]q
)

var _ = unsafe.Pointer(nil)

// --- begin cgo helpers ---

//export CGoPy_GoString
func CGoPy_GoString(str *C.char) string { 
	return C.GoString(str)
}

//export CGoPy_CString
func CGoPy_CString(s string) *C.char {
	return C.CString(s)
}

// --- end cgo helpers ---
`
)

type goGen struct {
	*printer

	fset *token.FileSet
	pkg  *Package
	err  ErrorList
}

func (g *goGen) gen() error {

	g.genPreamble()

	scope := g.pkg.pkg.Scope()
	names := scope.Names()
	for _, name := range names {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			debugf("ignore %q (not exported)\n", name)
			continue
		}
		debugf("processing %q...\n", name)

		switch obj := obj.(type) {
		case *types.Const:
			// TODO(sbinet)
			panic(fmt.Errorf("not yet supported: %v (%T)", obj, obj))
		case *types.Var:
			// TODO(sbinet)
			panic(fmt.Errorf("not yet supported: %v (%T)", obj, obj))

		case *types.Func:
			g.genFunc(obj)

		case *types.TypeName:
			named := obj.Type().(*types.Named)
			switch typ := named.Underlying().(type) {
			case *types.Struct:
				g.genStruct(obj, typ)

			case *types.Interface:
				// TODO(sbinet)
				panic(fmt.Errorf("not yet supported: %v (%T)", typ, obj))
			default:
				// TODO(sbinet)
				panic(fmt.Errorf("not yet supported: %v (%T)", typ, obj))
			}

		default:
			// TODO(sbinet)
			panic(fmt.Errorf("not yet supported: %v (%T)", obj, obj))

		}
	}

	g.Printf("// buildmode=c-shared needs a 'main'\n\nfunc main() {}\n")
	g.Printf("// tickle cgo\nfunc init() {\n")
	g.Indent()
	g.Printf("str := C.CString(%q)\n", g.pkg.Name())
	g.Printf("C.free(unsafe.Pointer(str))\n")
	g.Outdent()
	g.Printf("}\n")

	if len(g.err) > 0 {
		return g.err
	}

	return nil
}

func (g *goGen) genFunc(o *types.Func) {
	sig := o.Type().(*types.Signature)

	params := "(" + g.tupleString(sig.Params()) + ")"
	ret := g.tupleString(sig.Results())
	if sig.Results().Len() > 1 {
		ret = "(" + ret + ") "
	} else {
		ret += " "
	}

	//funcName := o.Name()
	g.Printf(`
//export GoPy_%[1]s
// GoPy_%[1]s wraps %[2]s
func GoPy_%[1]s%[3]v%[4]v{
`,
		o.Name(), o.FullName(),
		params,
		ret,
	)

	g.Indent()
	g.genFuncBody(o)
	g.Outdent()
	g.Printf("}\n\n")
}

func (g *goGen) genFuncBody(o *types.Func) {

	sig := o.Type().(*types.Signature)
	results := newVars(sig.Results())
	for i := range results {
		if i > 0 {
			g.Printf(", ")
		}
		g.Printf("_gopy_%03d", i)
	}
	if len(results) > 0 {
		g.Printf(" := ")
	}

	g.Printf("%s.%s(", g.pkg.Name(), o.Name())

	args := sig.Params()
	for i := 0; i < args.Len(); i++ {
		arg := args.At(i)
		tail := ""
		if i+1 < args.Len() {
			tail = ", "
		}
		g.Printf("%s%s", arg.Name(), tail)
	}
	g.Printf(")\n")

	if len(results) <= 0 {
		return
	}

	g.Printf("return ")
	for i, res := range results {
		if i > 0 {
			g.Printf(", ")
		}
		// if needWrap(res.GoType()) {
		// 	g.Printf("")
		// }
		if res.needWrap() {
			g.Printf("%s(unsafe.Pointer(&", res.dtype.cgotype)
		}
		g.Printf("_gopy_%03d /* %#v %v */", i, res, res.GoType().Underlying())
		if res.needWrap() {
			g.Printf("))")
		}
	}
	g.Printf("\n")
}

func (g *goGen) genStruct(obj *types.TypeName, typ *types.Struct) {
	//fmt.Printf("obj: %#v\ntyp: %#v\n", obj, typ)
	pkgname := obj.Pkg().Name()
	g.Printf("//export GoPy_%[1]s\n", obj.Name())
	g.Printf("type GoPy_%[1]s unsafe.Pointer\n\n", obj.Name())

	for i := 0; i < typ.NumFields(); i++ {
		f := typ.Field(i)
		if !f.Exported() {
			continue
		}

		ft := f.Type()
		ftname := g.qualifiedType(ft)
		if needWrapType(ft) {
			ftname = fmt.Sprintf("GoPy_%[1]s_field_%d", obj.Name(), i+1)
			g.Printf("//export %s\n", ftname)
			g.Printf("type %s unsafe.Pointer\n\n", ftname)
		}

		g.Printf("//export GoPy_%[1]s_getter_%[2]d\n", obj.Name(), i+1)
		g.Printf("func GoPy_%[1]s_getter_%[2]d(self GoPy_%[1]s) %[3]s {\n",
			obj.Name(), i+1,
			ftname,
		)
		g.Indent()
		g.Printf(
			"ret := (*%[1]s)(unsafe.Pointer(self))\n",
			pkgname+"."+obj.Name(),
		)

		if needWrapType(f.Type()) {
			dt := getTypedesc(f.Type())
			g.Printf("%s(unsafe.Pointer(&ret.%s))\n", dt.cgotype, f.Name())
		} else {
			g.Printf("return ret.%s\n", f.Name())
		}
		g.Outdent()
		g.Printf("}\n\n")
	}

	g.Printf("//export GoPy_%[1]s_new\n", obj.Name())
	g.Printf("func GoPy_%[1]s_new() GoPy_%[1]s {\n", obj.Name())
	g.Indent()
	g.Printf("return (GoPy_%[1]s)(unsafe.Pointer(&%[2]s.%[1]s{}))\n",
		obj.Name(),
		pkgname,
	)
	g.Outdent()
	g.Printf("}\n\n")
}

func (g *goGen) genPreamble() {
	n := g.pkg.pkg.Name()
	g.Printf(goPreamble, n, g.pkg.pkg.Path(), filepath.Base(n))
}

func (g *goGen) tupleString(tuple *types.Tuple) string {
	n := tuple.Len()
	if n <= 0 {
		return ""
	}

	str := make([]string, 0, n)
	for i := 0; i < tuple.Len(); i++ {
		v := tuple.At(i)
		n := v.Name()
		typ := v.Type()
		str = append(str, n+" "+g.qualifiedType(typ))
	}

	return strings.Join(str, ", ")
}

func (g *goGen) qualifiedType(typ types.Type) string {
	switch typ := typ.(type) {
	case *types.Basic:
		return typ.Name()
	case *types.Named:
		obj := typ.Obj()
		//return obj.Pkg().Name() + "." + obj.Name()
		return "GoPy_" + obj.Name()
		switch typ := typ.Underlying().(type) {
		case *types.Struct:
			return typ.String()
		default:
			return "GoPy_ooops_" + obj.Name()
		}
	}

	return fmt.Sprintf("%#T", typ)
}
