package test_utils

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func Assert(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func AssertSlicesEqual[T fmt.Stringer](t *testing.T, expected []T, got []T) {
	t.Helper()
	if !reflect.DeepEqual(expected, got) {
		error := "Expected:\n\t"
		for _, item := range expected {
			error += strings.ReplaceAll(item.String(), "\n", "\n\t")
		}
		error += "Got:\n\t"
		for _, item := range got {
			error += strings.ReplaceAll(item.String(), "\n", "\n\t")
		}
		t.Fatal(error)
	}
}

func AssertStringSlicesEqual(t *testing.T, expected []string, got []string) {
	t.Helper()
	if !reflect.DeepEqual(expected, got) {
		error := "Expected:"
		sep := ""
		for _, item := range expected {
			error += sep
			error += strings.ReplaceAll(item, "\n", "\n\t")
			sep = ", "
		}
		sep = ""
		error += "\n\tGot:"
		for _, item := range got {
			error += sep
			error += strings.ReplaceAll(item, "\n", "\n\t")
			sep = ", "
		}
		t.Fatal(error)
	}
}
