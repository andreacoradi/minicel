package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"regexp"
	"strconv"
	"strings"
)

type Cell struct {
	Content string
	Type    CellType
}

type CellType int

//go:generate stringer -type=CellType
const (
	Empty CellType = iota
	Text
	Number
	Expression
	Clone
)

type Table [][]Cell

var debugFlag = flag.Bool("debug", false, "enable intermediate representation and other debug infos")
var prettyPrintFlag = flag.Bool("pp", false, "pretty prints the cells with padding in-between")
var numberFormatVar = flag.String("format", "%.2f", "printf-like formatting for floating point numbers inside cells")

func init() {
	flag.Parse()
}

func main() {
	if len(flag.Args()) < 1 {
		log.Panic("Not enough arguments")
	}
	c, err := ioutil.ReadFile(flag.Arg(0))
	if err != nil {
		log.Panic(err)
	}

	// Calculate size
	content := strings.TrimSpace(string(c))
	table := parseTable(content)

	// Resolve cloning
	for i, row := range table {
		for j, cell := range row {
			switch cell.Type {
			case Clone:
				var targetCell Cell
				dir := charToDir[cell.Content[1]]
				incNumber := false
				var inc int
				if dir == Up || dir == Down {
					incNumber = true
				}
				switch dir {
				case Up:
					targetCell = table[i-1][j]
					inc = 1
				case Right:
					targetCell = table[i][j+1]
					inc = -1
				case Down:
					targetCell = table[i+1][j]
					inc = -1
				case Left:
					targetCell = table[i][j-1]
					inc = 1
				}

				if targetCell.Type == Expression {
					r, _ := regexp.Compile(`[A-Z]\d+`)
					matches := r.FindAllString(targetCell.Content, -1)

					for _, m := range matches {
						letter := m[0]
						number, err := strconv.Atoi(m[1:])
						if err != nil {
							log.Panic(err)
						}

						if incNumber {
							number += inc
						} else {
							if (letter < 'A' && inc < 0) || (letter > 'Z' && inc > 0) {
								log.Panic("Out of bounds")
							}
							letter += byte(inc)
						}

						targetCell.Content = strings.ReplaceAll(targetCell.Content, m, fmt.Sprintf("%s%d", string(letter), number))
					}
				}
				table[i][j] = targetCell
			}
		}
	}

	if *debugFlag {
		dumpTable(table)
		fmt.Println(strings.Repeat("-", 80))
	}

	// Final evaluation
	for i, row := range table {
		for j, cell := range row {
			switch cell.Type {
			case Expression:
				expr, err := parser.ParseExpr(cell.Content[1:])
				if err != nil {
					log.Panic(err)
				}

				value := parseExpr(table, expr)

				table[i][j] = Cell{
					Content: fmt.Sprintf(*numberFormatVar, value),
					Type:    Number,
				}
			case Clone:
				log.Panic("There should be no Clones after initial evaluation")
			}
		}
	}

	dumpTable(table)
}

func parseTable(content string) Table {
	size := len(strings.Split(content, "\n"))

	if *debugFlag {
		fmt.Println("Rows:", size)
	}

	table := make(Table, size)
	for i, row := range strings.Split(content, "\n") {
		parts := strings.Split(row, "|")
		for _, p := range parts {
			part := strings.TrimSpace(p)

			// FIXME: Find a way to eliminate empty cell rows or columns
			var t CellType

			if strings.HasPrefix(part, "=") {
				t = Expression
			} else if strings.HasPrefix(part, ":") {
				t = Clone
			} else if value, err := strconv.ParseFloat(part, 64); err == nil {
				t = Number
				part = fmt.Sprintf(*numberFormatVar, value)
			} else if matched, _ := regexp.MatchString(`[A-Z]`, part); matched {
				t = Text
			}

			table[i] = append(table[i], Cell{
				Content: part,
				Type:    t,
			})
		}
	}

	return table
}

func parseExpr(table Table, expr ast.Expr) float64 {
	if ident, ok := expr.(*ast.Ident); ok {
		cell, err := getCell(table, ident)
		if err != nil {
			log.Panic(err)
		}

		if cell.Type == Text {
			log.Panic("Text cell should not be used inside expressions")
		}
		return parseNumber(cell.Content)
	}

	if binaryExpr, ok := expr.(*ast.BinaryExpr); ok {
		lhs := parseExpr(table, binaryExpr.X)
		rhs := parseExpr(table, binaryExpr.Y)

		switch binaryExpr.Op {
		case token.ADD:
			return lhs + rhs
		case token.SUB:
			return lhs - rhs
		case token.MUL:
			return lhs * rhs
		case token.QUO:
			return lhs / rhs
		}
	}

	if number, ok := expr.(*ast.BasicLit); ok {
		return parseNumber(number.Value)
	}

	log.Panic("couldn't parse expr")
	return -1
}

func dumpTable(table Table) {
	// Estimate column widths
	widths := make([]int, len(table[0]))
	for j := 0; j < len(table[0]); j++ {
		var max int
		for i := 0; i < len(table); i++ {
			col := table[i][j]
			if len(col.Content) > max {
				max = len(col.Content)
			}
		}
		widths[j] = max
	}

	if *debugFlag {
		fmt.Println("Column widths:", widths)
	}

	// Render table
	for _, row := range table {
		for j, cell := range row {
			fmt.Print(cell.Content)
			if j < len(row)-1 {
				fmt.Print(strings.Repeat(" ", widths[j]-len(cell.Content)))

				if *prettyPrintFlag {
					fmt.Print(" | ")
				} else {
					fmt.Print("|")
				}
			}
		}
		fmt.Println()
	}
}

func getCell(table Table, ident *ast.Ident) (Cell, error) {
	letter := ident.Name[0]
	number, err := strconv.Atoi(ident.Name[1:])
	if err != nil {
		return Cell{}, err
	}

	if (letter-'A') < 0 || number < 0 {
		return Cell{}, fmt.Errorf("invalid cell identifier %q", ident.Name)
	}

	cell := table[number][letter-'A']
	return cell, nil
}

func parseNumber(s string) float64 {
	value, err := strconv.ParseFloat(s, 64)
	if err != nil {
		log.Panic(err)
	}
	return value
}
