package main

import (
	"fmt"
	"go/types"
	"strings"
)

func (g *generator) emptyCycleSource() string {
	var b strings.Builder
	for id, typ := range g.types {
		name := g.typeText(typ)
		base := types.Unalias(typ)
		if named, ok := base.(*types.Named); ok {
			base = named.Underlying()
		}
		fmt.Fprintf(&b, "func yamlvalidatorEmpty%d(value %s) bool {", id, name)
		if _, ok := base.(*types.Pointer); ok {
			b.WriteString("if value==nil{return true};")
		}
		b.WriteString("if zero,ok:=any(value).(interface{IsZero()bool});ok{return zero.IsZero()};")
		switch u := base.(type) {
		case *types.Basic:
			switch u.Kind() {
			case types.String:
				b.WriteString("return value==\"\"")
			case types.Bool:
				b.WriteString("return !bool(value)")
			default:
				b.WriteString("return value==0")
			}
		case *types.Map, *types.Slice:
			b.WriteString("return len(value)==0")
		case *types.Pointer, *types.Interface:
			b.WriteString("return value==nil")
		case *types.Struct:
			for _, entry := range g.schemaFields[id] {
				f := entry.field
				fmt.Fprintf(&b, "if !yamlvalidatorEmpty%d(value.%s){return false};", f.id, f.name)
			}
			b.WriteString("return true")
		default:
			b.WriteString("return false")
		}
		b.WriteString("}\n")
		fmt.Fprintf(&b, "func yamlvalidatorCycle%d(value %s,ctx *yamlvalidator.GeneratedCycleContext,depth int)error{done,err:=ctx.Enter(value,depth);if err!=nil{return err};defer done();", id, name)
		if customCodec(typ) && !g.rootOrPointerType(typ) {
			b.WriteString("return nil}\n")
			continue
		}
		switch u := base.(type) {
		case *types.Pointer:
			fmt.Fprintf(&b, "if value==nil{return nil};return yamlvalidatorCycle%d(*value,ctx,depth+1)", g.schemaID(u.Elem()))
		case *types.Struct:
			for _, entry := range g.schemaFields[id] {
				f := entry.field
				if f.omit {
					fmt.Fprintf(&b, "if !yamlvalidatorEmpty%d(value.%s){", f.id, f.name)
				}
				fmt.Fprintf(&b, "if err:=yamlvalidatorCycle%d(value.%s,ctx,depth+1);err!=nil{return err};", f.id, f.name)
				if f.omit {
					b.WriteString("};")
				}
			}
			b.WriteString("return nil")
		case *types.Slice:
			fmt.Fprintf(&b, "for _,item:=range value{if err:=yamlvalidatorCycle%d(item,ctx,depth+1);err!=nil{return err}};return nil", g.schemaID(u.Elem()))
		case *types.Array:
			fmt.Fprintf(&b, "for _,item:=range value{if err:=yamlvalidatorCycle%d(item,ctx,depth+1);err!=nil{return err}};return nil", g.schemaID(u.Elem()))
		case *types.Map:
			fmt.Fprintf(&b, "for _,item:=range value{if err:=yamlvalidatorCycle%d(item,ctx,depth+1);err!=nil{return err}};return nil", g.schemaID(u.Elem()))
		case *types.Interface:
			b.WriteString("return ctx.CheckDynamic(value,depth)")
		default:
			b.WriteString("return nil")
		}
		b.WriteString("}\n")
	}
	return b.String()
}
