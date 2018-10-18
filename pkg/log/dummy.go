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

// Dummy provides a dummy logger implementation
type Dummy struct{}

// New creates a Dummy logger
func (z Dummy) New(level Level) (Logger, error) {
	return &Dummy{}, nil
}

// WithFields returns a clone of the Dummy logger
func (z Dummy) WithFields(fields []Field, prefix string, suffix string) Logger {
	return &Dummy{}
}

// Debug logs with level Debug
func (z *Dummy) Debug(args ...interface{}) {}

// Debugf formats and logs with level Debug
func (z *Dummy) Debugf(template string, args ...interface{}) {}

// Info logs with level Info
func (z *Dummy) Info(args ...interface{}) {}

// Infof formats and logs with level Info
func (z *Dummy) Infof(template string, args ...interface{}) {}

// Warn logs with level Warn
func (z *Dummy) Warn(args ...interface{}) {}

// Warnf formats and logs with level Warn
func (z *Dummy) Warnf(template string, args ...interface{}) {}

// Warningf formats and logs with level Warn
func (z *Dummy) Warningf(template string, args ...interface{}) {}

// Error logs with level Error
func (z *Dummy) Error(args ...interface{}) {}

// Errorf formats and logs with level Error
func (z *Dummy) Errorf(template string, args ...interface{}) {}
