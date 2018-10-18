/*
Copyright 2018 Expedia Inc.

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

package log

import "fmt"

// Level is a pseudo enum for setting log levels
type Level int

// Sets the accepted log levels
const (
	INFO Level = iota + 1
	DEBUG
)

// Levels defines a pseudo enum of log levels
var Levels = [...]string{
	"INFO",
	"DEBUG",
}

// String converts a log level to a string
func (ll Level) String() string {
	return Levels[ll]
}

// StringToLogLevel converts a log level in string format to the corresponding int value
func StringToLogLevel(l string) (Level, error) {
	for i, v := range Levels {
		if v == l {
			return Level(i + 1), nil
		}
	}
	return Level(INFO), fmt.Errorf("invalid log level %q", l)
}

// Field defines a log field with a width
type Field struct {
	Field string
	Width uint
}

// Logger is an interface that needs to be implemented in order to log.
type Logger interface {
	Debug(...interface{})
	Debugf(string, ...interface{})

	Info(...interface{})
	Infof(string, ...interface{})

	Warn(...interface{})
	Warnf(string, ...interface{})
	Warningf(string, ...interface{})

	Error(...interface{})
	Errorf(string, ...interface{})

	New(Level) (Logger, error)
	WithFields(fields []Field, prefix string, suffix string) Logger
}
