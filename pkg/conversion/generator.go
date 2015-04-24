/*
Copyright 2015 Google Inc. All rights reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package conversion

import (
	"fmt"
	"io"
	"reflect"
	"strings"
)

type Generator interface {
	GenerateConversionsForType(version string, reflection reflect.Type) error
	WriteConversionFunctions(w io.Writer) error
	OverwritePackage(pkg, overwrite string)
}

func NewGenerator(scheme *Scheme) Generator {
	return &generator{
		scheme:        scheme,
		convertibles:  make(map[reflect.Type]reflect.Type),
		pkgOverwrites: make(map[string]string),
	}
}

var complexTypes []reflect.Kind = []reflect.Kind{reflect.Map, reflect.Ptr, reflect.Slice, reflect.Interface, reflect.Struct}

type generator struct {
	scheme       *Scheme
	convertibles map[reflect.Type]reflect.Type
	// If pkgOverwrites is set for a given package name, that package name
	// will be replaced while writing conversion function. If empty, package
	// name will be omitted.
	pkgOverwrites map[string]string
}

func (g *generator) GenerateConversionsForType(version string, reflection reflect.Type) error {
	kind := reflection.Name()
	internalObj, err := g.scheme.NewObject(g.scheme.InternalVersion, kind)
	if err != nil {
		return fmt.Errorf("cannot create an object of type %v in internal version", kind)
	}
	internalObjType := reflect.TypeOf(internalObj)
	if internalObjType.Kind() != reflect.Ptr {
		return fmt.Errorf("created object should be of type Ptr: ", internalObjType.Kind())
	}
	return g.generateConversionsBetween(reflection, internalObjType.Elem())
}

func (g *generator) generateConversionsBetween(inType, outType reflect.Type) error {
	// Avoid processing the same type multiple times.
	if value, found := g.convertibles[inType]; found {
		if value != outType {
			return fmt.Errorf("multiple possible convertibles for %v", inType)
		}
		return nil
	}
	if inType == outType {
		// Don't generate conversion methods for the same type.
		return nil
	}

	if inType.Kind() != outType.Kind() {
		return fmt.Errorf("cannot convert types of different kinds: %v %v", inType, outType)
	}
	switch inType.Kind() {
	case reflect.Map:
		return g.generateConversionsForMap(inType, outType)
	case reflect.Ptr:
		return g.generateConversionsBetween(inType.Elem(), outType.Elem())
	case reflect.Slice:
		return g.generateConversionsForSlice(inType, outType)
	case reflect.Interface:
		// TODO(wojtek-t): Currently we rely on default conversion functions for interfaces.
		// Add support for reflect.Interface.
		return nil
	case reflect.Struct:
		return g.generateConversionsForStruct(inType, outType)
	default:
		// All simple types should be handled correctly with default conversion.
		return nil
	}
	panic("This should never happen")
}

func isComplexType(reflection reflect.Type) bool {
	for _, complexType := range complexTypes {
		if complexType == reflection.Kind() {
			return true
		}
	}
	return false
}

func (g *generator) generateConversionsForMap(inType, outType reflect.Type) error {
	inKey := inType.Key()
	outKey := outType.Key()
	if err := g.generateConversionsBetween(inKey, outKey); err != nil {
		return err
	}
	inValue := inType.Elem()
	outValue := outType.Elem()
	if err := g.generateConversionsBetween(inValue, outValue); err != nil {
		return err
	}
	// We don't add it to g.convertibles - maps should be handled correctly
	// inside appropriate conversion functions.
	return nil
}

func (g *generator) generateConversionsForSlice(inType, outType reflect.Type) error {
	inElem := inType.Elem()
	outElem := outType.Elem()
	if err := g.generateConversionsBetween(inElem, outElem); err != nil {
		return err
	}
	// We don't add it to g.convertibles - slices should be handled correctly
	// inside appropriate conversion functions.
	return nil
}

func (g *generator) generateConversionsForStruct(inType, outType reflect.Type) error {
	for i := 0; i < inType.NumField(); i++ {
		inField := inType.Field(i)
		outField, found := outType.FieldByName(inField.Name)
		if !found {
			return fmt.Errorf("couldn't find a corresponding field %v in %v", inField.Name, outType)
		}
		if inField.Type.Kind() != outField.Type.Kind() {
			return fmt.Errorf("cannot convert types of different kinds: %v %v", inField, outField)
		}
		if isComplexType(inField.Type) {
			if err := g.generateConversionsBetween(inField.Type, outField.Type); err != nil {
				return err
			}
		}
	}
	g.convertibles[inType] = outType
	return nil
}

func (g *generator) WriteConversionFunctions(w io.Writer) error {
	indent := 2
	for inType, outType := range g.convertibles {
		// All types in g.convertibles are structs.
		if inType.Kind() != reflect.Struct {
			return fmt.Errorf("non-struct conversions are not-supported")
		}
		if err := g.writeConversionForStruct(w, inType, outType, indent); err != nil {
			return err
		}
		if err := g.writeConversionForStruct(w, outType, inType, indent); err != nil {
			return err
		}
	}
	return nil
}

func (g *generator) typeName(inType reflect.Type) string {
	switch inType.Kind() {
	case reflect.Map:
		return fmt.Sprintf("map[%s]%s", g.typeName(inType.Key()), g.typeName(inType.Elem()))
	case reflect.Slice:
		return fmt.Sprintf("[]%s", g.typeName(inType.Elem()))
	default:
		typeWithPkg := fmt.Sprintf("%s", inType)
		slices := strings.Split(typeWithPkg, ".")
		if len(slices) == 1 {
			// Default package.
			return slices[0]
		}
		if len(slices) == 2 {
			pkg := slices[0]
			if val, found := g.pkgOverwrites[pkg]; found {
				pkg = val
			}
			if pkg != "" {
				pkg = pkg + "."
			}
			return pkg + slices[1]
		}
		panic("Incorrect type name: " + typeWithPkg)
	}
}

func indentLine(w io.Writer, indent int) error {
	indentation := ""
	for i := 0; i < indent; i++ {
		indentation = indentation + "\t"
	}
	_, err := io.WriteString(w, indentation)
	return err
}

func writeLine(w io.Writer, indent int, line string) error {
	if err := indentLine(w, indent); err != nil {
		return err
	}
	if _, err := io.WriteString(w, line); err != nil {
		return err
	}
	return nil
}

func writeHeader(w io.Writer, inType, outType string, indent int) error {
	format := "func(in *%s, out *%s, s conversion.Scope) error {\n"
	stmt := fmt.Sprintf(format, inType, outType)
	return writeLine(w, indent, stmt)
}

func writeFooter(w io.Writer, indent int) error {
	if err := writeLine(w, indent+1, "return nil\n"); err != nil {
		return err
	}
	if err := writeLine(w, indent, "},\n"); err != nil {
		return err
	}
	return nil
}

func (g *generator) writeConversionForMap(w io.Writer, inField, outField reflect.StructField, indent int) error {
	ifFormat := "if in.%s != nil {\n"
	ifStmt := fmt.Sprintf(ifFormat, inField.Name)
	if err := writeLine(w, indent, ifStmt); err != nil {
		return err
	}
	makeFormat := "out.%s = make(%s)\n"
	makeStmt := fmt.Sprintf(makeFormat, outField.Name, g.typeName(outField.Type))
	if err := writeLine(w, indent+1, makeStmt); err != nil {
		return err
	}
	forFormat := "for key, val := range in.%s {\n"
	forStmt := fmt.Sprintf(forFormat, inField.Name)
	if err := writeLine(w, indent+1, forStmt); err != nil {
		return err
	}
	// Whether we need to explicitly create a new value.
	newValue := false
	if isComplexType(inField.Type.Elem()) || !inField.Type.Elem().ConvertibleTo(outField.Type.Elem()) {
		newValue = true
		newFormat := "newVal := %s{}\n"
		newStmt := fmt.Sprintf(newFormat, g.typeName(outField.Type.Elem()))
		if err := writeLine(w, indent+2, newStmt); err != nil {
			return err
		}
		convertStmt := "if err := s.Convert(&val, &newVal, 0); err != nil {\n"
		if err := writeLine(w, indent+2, convertStmt); err != nil {
			return err
		}
		if err := writeLine(w, indent+3, "return err\n"); err != nil {
			return err
		}
		if err := writeLine(w, indent+2, "}\n"); err != nil {
			return err
		}
	}
	if inField.Type.Key().ConvertibleTo(outField.Type.Key()) {
		value := "val"
		if newValue {
			value = "newVal"
		}
		assignStmt := ""
		if inField.Type.Key().AssignableTo(outField.Type.Key()) {
			assignStmt = fmt.Sprintf("out.%s[key] = %s\n", outField.Name, value)
		} else {
			assignStmt = fmt.Sprintf("out.%s[%s(key)] = %s\n", outField.Name, g.typeName(outField.Type.Key()), value)
		}
		if err := writeLine(w, indent+2, assignStmt); err != nil {
			return err
		}
	} else {
		// TODO(wojtek-t): Support maps with keys that are non-convertible to each other.
		return fmt.Errorf("conversions between unconvertible keys in map are not supported.")
	}
	if err := writeLine(w, indent+1, "}\n"); err != nil {
		return err
	}
	if err := writeLine(w, indent, "}\n"); err != nil {
		return err
	}
	return nil
}

func (g *generator) writeConversionForSlice(w io.Writer, inField, outField reflect.StructField, indent int) error {
	ifFormat := "if in.%s != nil {\n"
	ifStmt := fmt.Sprintf(ifFormat, inField.Name)
	if err := writeLine(w, indent, ifStmt); err != nil {
		return err
	}

	makeFormat := "out.%s = make(%s, len(in.%s))\n"
	makeStmt := fmt.Sprintf(makeFormat, outField.Name, g.typeName(outField.Type), inField.Name)
	if err := writeLine(w, indent+1, makeStmt); err != nil {
		return err
	}
	forFormat := "for i := range in.%s {\n"
	forStmt := fmt.Sprintf(forFormat, inField.Name)
	if err := writeLine(w, indent+1, forStmt); err != nil {
		return err
	}
	if inField.Type.Elem().AssignableTo(outField.Type.Elem()) {
		assignFormat := "out.%s[i] = in.%s[i]\n"
		assignStmt := fmt.Sprintf(assignFormat, outField.Name, inField.Name)
		if err := writeLine(w, indent+2, assignStmt); err != nil {
			return err
		}
	} else {
		assignFormat := "if err := s.Convert(&in.%s[i], &out.%s[i], 0); err != nil {\n"
		assignStmt := fmt.Sprintf(assignFormat, inField.Name, outField.Name)
		if err := writeLine(w, indent+2, assignStmt); err != nil {
			return err
		}
		if err := writeLine(w, indent+3, "return err\n"); err != nil {
			return err
		}
		if err := writeLine(w, indent+2, "}\n"); err != nil {
			return err
		}
	}
	if err := writeLine(w, indent+1, "}\n"); err != nil {
		return err
	}
	if err := writeLine(w, indent, "}\n"); err != nil {
		return err
	}
	return nil
}

func (g *generator) writeConversionForStruct(w io.Writer, inType, outType reflect.Type, indent int) error {
	if err := writeHeader(w, g.typeName(inType), g.typeName(outType), indent); err != nil {
		return err
	}
	for i := 0; i < inType.NumField(); i++ {
		inField := inType.Field(i)
		outField, _ := outType.FieldByName(inField.Name)

		switch inField.Type.Kind() {
		case reflect.Map, reflect.Ptr, reflect.Slice, reflect.Interface, reflect.Struct:
			// Don't copy these via assignment/conversion!
		default:
			// This should handle all simple types.
			if inField.Type.AssignableTo(outField.Type) {
				assignFormat := "out.%s = in.%s\n"
				assignStmt := fmt.Sprintf(assignFormat, outField.Name, inField.Name)
				if err := writeLine(w, indent+1, assignStmt); err != nil {
					return err
				}
				continue
			}
			if inField.Type.ConvertibleTo(outField.Type) {
				assignFormat := "out.%s = %s(in.%s)\n"
				assignStmt := fmt.Sprintf(assignFormat, outField.Name, g.typeName(outField.Type), inField.Name)
				if err := writeLine(w, indent+1, assignStmt); err != nil {
					return err
				}
				continue
			}
		}

		// If the field is a slice, copy its elements one by one.
		if inField.Type.Kind() == reflect.Slice {
			if err := g.writeConversionForSlice(w, inField, outField, indent+1); err != nil {
				return err
			}
			continue
		}

		// If the field is a map, copy its elements one by one.
		if inField.Type.Kind() == reflect.Map {
			if err := g.writeConversionForMap(w, inField, outField, indent+1); err != nil {
				return err
			}
			continue
		}

		// TODO(wojtek-t): At some point we may consider named functions and call
		// appropriate function here instead of using generic Convert method that
		// will call the same method underneath after some additional operations.

		assignFormat := "if err := s.Convert(&in.%s, &out.%s, 0); err != nil {\n"
		assignStmt := fmt.Sprintf(assignFormat, inField.Name, outField.Name)
		if err := writeLine(w, indent+1, assignStmt); err != nil {
			return err
		}
		if err := writeLine(w, indent+2, "return err\n"); err != nil {
			return err
		}
		if err := writeLine(w, indent+1, "}\n"); err != nil {
			return err
		}
	}
	if err := writeFooter(w, indent); err != nil {
		return err
	}
	return nil
}

func (g *generator) OverwritePackage(pkg, overwrite string) {
	g.pkgOverwrites[pkg] = overwrite
}
