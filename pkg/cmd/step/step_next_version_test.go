// +build unit

package step_test

import (
	"strings"
	"testing"

	step2 "github.com/jenkins-x/jx/pkg/cmd/opts/step"
	"github.com/jenkins-x/jx/pkg/cmd/step"
	"github.com/stretchr/testify/require"

	"github.com/jenkins-x/jx/pkg/cmd/opts"
	"github.com/stretchr/testify/assert"
	xmldom "github.com/subchen/go-xmldom"
)

func TestMakefile(t *testing.T) {
	t.Parallel()
	o := step.StepNextVersionOptions{
		StepOptions: step2.StepOptions{
			CommonOptions: &opts.CommonOptions{},
		},
		Dir:      "test_data/next_version/make",
		Filename: "Makefile",
	}

	v, err := o.GetVersion()

	assert.NoError(t, err)

	assert.Equal(t, "1.2.0-SNAPSHOT", v, "error with GetVersion for a Makefile")
}

func TestPomXML(t *testing.T) {
	t.Parallel()
	o := step.StepNextVersionOptions{
		StepOptions: step2.StepOptions{
			CommonOptions: &opts.CommonOptions{},
		},
		Dir:      "test_data/next_version/java",
		Filename: "pom.xml",
	}

	v, err := o.GetVersion()

	assert.NoError(t, err)

	assert.Equal(t, "1.0-SNAPSHOT", v, "error with GetVersion for a pom.xml")
}

func TestChart(t *testing.T) {
	t.Parallel()
	o := step.StepNextVersionOptions{
		StepOptions: step2.StepOptions{
			CommonOptions: &opts.CommonOptions{},
		},
		Dir:      "test_data/next_version/helm",
		Filename: "Chart.yaml",
	}

	v, err := o.GetVersion()

	assert.NoError(t, err)

	assert.Equal(t, "0.0.1-SNAPSHOT", v, "error with GetVersion for a Chart.yaml")
}

func TestReplacePomXmlVersion(t *testing.T) {
	t.Parallel()

	data := []byte(`
<?xml version="1.0" encoding="UTF-8"?>
<project xmlns="http://maven.apache.org/POM/4.0.0" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
	xsi:schemaLocation="http://maven.apache.org/POM/4.0.0 https://maven.apache.org/xsd/maven-4.0.0.xsd">
	<modelVersion>4.0.0</modelVersion>
	<parent>
		<groupId>org.springframework.boot</groupId>
		<artifactId>spring-boot-starter-parent</artifactId>
		<version>2.2.5.RELEASE</version>
		<relativePath/> <!-- lookup parent from repository -->
	</parent>
	<groupId>com.example</groupId>
	<artifactId>demo300</artifactId>
	<version>0.0.1-SNAPSHOT</version>
	<name>demo</name>
</project>`)

	newVersion := "2.0"
	actual, err := step.ReplacePomXmlVersion(data, newVersion)
	require.NoError(t, err, "failed to replace pom version")
	assert.NotEmpty(t, actual, "no xml returned")

	t.Logf("generated XML: %s", actual)
	doc, err := xmldom.Parse(strings.NewReader(actual))

	version := doc.Root.GetChild("version")
	require.NotNil(t, version, "no <version> element found")
	assert.Equal(t, newVersion, version.Text, "<version> element text")
}
