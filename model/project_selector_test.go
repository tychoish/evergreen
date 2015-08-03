package model

import (
	"fmt"
	. "github.com/smartystreets/goconvey/convey"
	"testing"
)

// helper for comparing a selector string with its expected output
func selectorShouldParse(s string, expected Selector) {
	Convey(fmt.Sprintf(`selector string "%v" should parse correctly`, s), func() {
		So(ParseSelector(s), ShouldResemble, expected)
	})
}

func TestBasicSelector(t *testing.T) {
	Convey("With a set of test selection strings", t, func() {

		Convey("single selectors should parse", func() {
			selectorShouldParse("myTask", Selector{{name: "myTask"}})
			selectorShouldParse("!myTask", Selector{{name: "myTask", negated: true}})
			selectorShouldParse(".myTag", Selector{{name: "myTag", tag: true}})
			selectorShouldParse("!.myTag", Selector{{name: "myTag", tag: true, negated: true}})
		})

		Convey("multi-selectors should parse", func() {
			selectorShouldParse(".tag1 .tag2", Selector{
				{name: "tag1", tag: true},
				{name: "tag2", tag: true},
			})
			selectorShouldParse(".tag1 !.tag2", Selector{
				{name: "tag1", tag: true},
				{name: "tag2", tag: true, negated: true},
			})
			selectorShouldParse("!.tag1 .tag2", Selector{
				{name: "tag1", tag: true, negated: true},
				{name: "tag2", tag: true},
			})
			selectorShouldParse(".mytag !mytask", Selector{
				{name: "mytag", tag: true},
				{name: "mytask", negated: true},
			})
			selectorShouldParse(".tag1 .tag2 .tag3 !.tag4", Selector{
				{name: "tag1", tag: true},
				{name: "tag2", tag: true},
				{name: "tag3", tag: true},
				{name: "tag4", tag: true, negated: true},
			})

			Convey("selectors with unusual whitespace should parse", func() {
				selectorShouldParse("    .myTag   ", Selector{{name: "myTag", tag: true}})
				selectorShouldParse(".mytag\t\t!mytask", Selector{
					{name: "mytag", tag: true},
					{name: "mytask", negated: true},
				})
				selectorShouldParse("\r\n.mytag\r\n!mytask\n", Selector{
					{name: "mytag", tag: true},
					{name: "mytask", negated: true},
				})
			})
		})
	})
}
