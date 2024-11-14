/*
   Copyright 2020 Docker Compose CLI authors

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

package helper

import (
	"fmt"
	"strconv"
	"sync"
)

// ColorFunc use ANSI codes to render colored text on console
type ColorFunc func(s string) string

var names = []string{
	"grey",
	"red",
	"green",
	"yellow",
	"blue",
	"magenta",
	"cyan",
	"white",
}

var rainbow []ColorFunc

var ErrorLoggerColorFunc ColorFunc
var InfoLoggerColorFunc ColorFunc
var DebugLoggerColorFunc ColorFunc

func init() {
	colors := map[string]ColorFunc{}
	for i, name := range names {
		colors[name] = makeColorFunc(strconv.Itoa(30 + i))
		colors["intense_"+name] = makeColorFunc(strconv.Itoa(30+i) + ";1")
	}
	rainbow = []ColorFunc{
		colors["cyan"],
		colors["yellow"],
		colors["green"],
		colors["magenta"],
		colors["blue"],
		colors["intense_cyan"],
		colors["intense_yellow"],
		colors["intense_green"],
		colors["intense_magenta"],
		colors["intense_blue"],
	}
	ErrorLoggerColorFunc = colors["red"]
	InfoLoggerColorFunc = colors["white"]
	DebugLoggerColorFunc = colors["intense_white"]
}

func makeColorFunc(code string) ColorFunc {
	return func(s string) string {
		return ansiColor(code, s)
	}
}

func ansiColor(code, s string, formatOpts ...string) string {
	return fmt.Sprintf("%s%s%s", ansiColorCode(code, formatOpts...), s, ansiColorCode("0"))
}

// Everything about ansiColorCode color https://hyperskill.org/learn/step/18193
func ansiColorCode(code string, formatOpts ...string) string {
	res := "\033["
	for _, c := range formatOpts {
		res = fmt.Sprintf("%s%s;", res, c)
	}
	return fmt.Sprintf("%s%sm", res, code)
}

var NextColor = rainbowColor
var currentIndex = 0
var mutex sync.Mutex

func rainbowColor() ColorFunc {
	mutex.Lock()
	defer mutex.Unlock()
	result := rainbow[currentIndex]
	currentIndex = (currentIndex + 1) % len(rainbow)
	return result
}
