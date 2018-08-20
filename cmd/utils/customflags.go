package utils

import (
	"encoding"
	"errors"
	"flag"
	"fmt"
	"github.com/entropyio/go-entropy/common/mathutil"
	"gopkg.in/urfave/cli.v1"
	"math/big"
	"os"
	"os/user"
	"path"
	"strings"
)

// ----- DirectoryString -----
type DirectoryString struct {
	Value string
}

// implement cli.Flag
func (ds *DirectoryString) String() string {
	return ds.Value
}

// implement cli.Flag
func (ds *DirectoryString) Set(value string) error {
	ds.Value = expandPath(value)
	return nil
}

// ------ DirectoryString ------

// ------- textMarshalerVal ---------
// textMarshalerVal turns a TextMarshaler into a flag.Value
type textMarshalerVal struct {
	v TextMarshaler
}

func (v textMarshalerVal) String() string {
	if v.v == nil {
		return ""
	}
	text, _ := v.v.MarshalText()
	return string(text)
}

func (v textMarshalerVal) Set(s string) error {
	return v.v.UnmarshalText([]byte(s))
}

// ------- textMarshalerVal ---------

// ----- TextMarshaler --------
type TextMarshaler interface {
	encoding.TextMarshaler
	encoding.TextUnmarshaler
}

// TextMarshalerFlag wraps a TextMarshaler value.
type TextMarshalerFlag struct {
	Name  string
	Value TextMarshaler
	Usage string
}

func (f TextMarshalerFlag) GetName() string {
	return f.Name
}

func (f TextMarshalerFlag) String() string {
	return fmt.Sprintf("%s \"%v\"\t%v", prefixedNames(f.Name), f.Value, f.Usage)
}

func (f TextMarshalerFlag) Apply(set *flag.FlagSet) {
	eachName(f.Name, func(name string) {
		set.Var(textMarshalerVal{f.Value}, f.Name, f.Usage)
	})
}

// ----- TextMarshaler --------

// bigValue turns *big.Int into a flag.Value
type bigValue big.Int

func (b *bigValue) String() string {
	if b == nil {
		return ""
	}
	return (*big.Int)(b).String()
}

func (b *bigValue) Set(s string) error {
	intNum, ok := mathutil.ParseBig256(s)
	if !ok {
		return errors.New("invalid integer syntax")
	}
	*b = (bigValue)(*intNum)
	return nil
}

// GlobalTextMarshaler returns the value of a TextMarshalerFlag from the global flag set.
func GlobalTextMarshaler(ctx *cli.Context, name string) TextMarshaler {
	val := ctx.GlobalGeneric(name)
	if val == nil {
		return nil
	}
	return val.(textMarshalerVal).v
}

// BigFlag is a command line flag that accepts 256 bit big integers in decimal or
// hexadecimal syntax.
type BigFlag struct {
	Name  string
	Value *big.Int
	Usage string
}

// GlobalBig returns the value of a BigFlag from the global flag set.
func GlobalBig(ctx *cli.Context, name string) *big.Int {
	val := ctx.GlobalGeneric(name)
	if val == nil {
		return nil
	}
	return (*big.Int)(val.(*bigValue))
}

// Expands a file path
// 1. replace tilde with users home dir
// 2. expands embedded environment variables
// 3. cleans the path, e.g. /a/b/../c -> /a/c
// Note, it has limitations, e.g. ~someuser/tmp will not be expanded
func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") || strings.HasPrefix(p, "~\\") {
		if home := homeDir(); home != "" {
			p = home + p[1:]
		}
	}
	return path.Clean(os.ExpandEnv(p))
}

func homeDir() string {
	if home := os.Getenv("HOME"); home != "" {
		return home
	}
	if usr, err := user.Current(); err == nil {
		return usr.HomeDir
	}
	return ""
}

// ---------

type DirectoryFlag struct {
	Name  string
	Value DirectoryString
	Usage string
}

// implement cli.Flag
func (flag DirectoryFlag) String() string {
	fmtString := "%s %v\t%v"
	if len(flag.Value.Value) > 0 {
		fmtString = "%s \"%v\"\t%v"
	}
	return fmt.Sprintf(fmtString, prefixedNames(flag.Name), flag.Value.Value, flag.Usage)
}

// implement cli.Flag
func (flag DirectoryFlag) GetName() string {
	return flag.Name
}

// implement cli.Flag
func (flag *DirectoryFlag) Set(value string) {
	flag.Value.Value = value
}

// called by cli library, grabs variable from environment (if in env)
// and adds variable to flag set for parsing.
func (flag DirectoryFlag) Apply(set *flag.FlagSet) {
	eachName(flag.Name, func(name string) {
		set.Var(&flag.Value, flag.Name, flag.Usage)
	})
}

func eachName(longName string, fn func(string)) {
	parts := strings.Split(longName, ",")
	for _, name := range parts {
		name = strings.Trim(name, " ")
		fn(name)
	}
}

func prefixedNames(fullName string) (prefixed string) {
	parts := strings.Split(fullName, ",")
	for i, name := range parts {
		name = strings.Trim(name, " ")
		prefixed += prefixFor(name) + name
		if i < len(parts)-1 {
			prefixed += ", "
		}
	}
	return
}

func prefixFor(name string) (prefix string) {
	if len(name) == 1 {
		prefix = "-"
	} else {
		prefix = "--"
	}

	return
}

// --------
