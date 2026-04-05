package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

const TokenToCharRatio float64 = 3.4

type Region struct {
	Name   string `json:"name"`
	Tokens int    `json:"tokens"`
}

type Node struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"` // file | directory
	Tokens   int      `json:"tokens,omitempty"`
	Regions  []Region `json:"regions,omitempty"`
	Children []*Node  `json:"children,omitempty"`
}

func analyzeFile(path string) ([]Region, int, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	var regions []Region
	var current *Region
	totalChars := 0
	hasRegion := false

	for scanner.Scan() {
		line := scanner.Text()
		lineChars := len([]rune(line)) + 1
		totalChars += lineChars

		trim := strings.TrimSpace(line)

		if strings.HasPrefix(trim, "// region") {
			name := strings.TrimSpace(strings.TrimPrefix(trim, "// region"))
			current = &Region{Name: name}
			hasRegion = true
			continue
		}

		if strings.HasPrefix(trim, "// endregion") {
			if current != nil {
				current.Tokens = int(float64(current.Tokens) / TokenToCharRatio)
				regions = append(regions, *current)
				current = nil
			}
			continue
		}

		if current != nil {
			current.Tokens += lineChars
		}
	}

	if !hasRegion {
		return nil, int(float64(totalChars) / TokenToCharRatio), nil
	}

	for i := range regions {
		regions[i].Tokens = int(float64(regions[i].Tokens) / TokenToCharRatio)
	}

	return regions, int(float64(totalChars) / TokenToCharRatio), nil
}

func buildTree(root string) (*Node, error) {
	rootNode := &Node{
		Name: filepath.Base(root),
		Type: "directory",
	}

	nodeMap := map[string]*Node{
		root: rootNode,
	}

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {

		if err != nil {
			return err
		}

		if path == root {
			return nil
		}

		// skip hidden dirs
		if info.IsDir() && strings.HasPrefix(info.Name(), ".") {
			return filepath.SkipDir
		}

		parent := filepath.Dir(path)
		parentNode := nodeMap[parent]

		node := &Node{
			Name: info.Name(),
		}

		if info.IsDir() {
			node.Type = "directory"
			nodeMap[path] = node
		} else {
			node.Type = "file"

			regions, tokens, err := analyzeFile(path)
			if err == nil {
				node.Tokens = tokens
				if regions != nil {
					node.Regions = regions
				}
			}
		}

		parentNode.Children = append(parentNode.Children, node)

		return nil
	})

	if err != nil {
		return nil, err
	}

	return rootNode, nil
}

func main() {

	root := "."

	tree, err := buildTree(root)
	if err != nil {
		panic(err)
	}

	err = os.MkdirAll(".af", 0755)
	if err != nil {
		panic(err)
	}

	file, err := os.Create(".af/map.json")
	if err != nil {
		panic(err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	err = encoder.Encode(tree)
	if err != nil {
		panic(err)
	}
}
