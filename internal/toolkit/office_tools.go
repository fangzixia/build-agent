package toolkit

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	docx "github.com/fumiama/go-docx"
	gofpdf "github.com/go-pdf/fpdf"
	"github.com/ledongthuc/pdf"
	"github.com/xuri/excelize/v2"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

// ==================== Word ====================

type readWordInput struct {
	Path string `json:"path"`
}
type readWordOutput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func readWordTool(workspaceRoot string) (tool.BaseTool, error) {
	fn := func(_ context.Context, in readWordInput) (*readWordOutput, error) {
		target, err := resolveSafePath(workspaceRoot, in.Path)
		if err != nil {
			return nil, err
		}
		f, err := os.Open(target)
		if err != nil {
			return nil, fmt.Errorf("open docx: %w", err)
		}
		defer f.Close()
		fi, err := f.Stat()
		if err != nil {
			return nil, err
		}
		doc, err := docx.Parse(f, fi.Size())
		if err != nil {
			return nil, fmt.Errorf("parse docx: %w", err)
		}
		var sb strings.Builder
		for _, item := range doc.Document.Body.Items {
			switch v := item.(type) {
			case *docx.Paragraph:
				for _, child := range v.Children {
					if run, ok := child.(*docx.Run); ok {
						for _, rc := range run.Children {
							if t, ok := rc.(*docx.Text); ok {
								sb.WriteString(t.Text)
							}
						}
					}
				}
				sb.WriteString("\n")
			case *docx.Table:
				for _, row := range v.TableRows {
					for i, cell := range row.TableCells {
						for _, para := range cell.Paragraphs {
							for _, child := range para.Children {
								if run, ok := child.(*docx.Run); ok {
									for _, rc := range run.Children {
										if t, ok := rc.(*docx.Text); ok {
											sb.WriteString(t.Text)
										}
									}
								}
							}
						}
						if i < len(row.TableCells)-1 {
							sb.WriteString("\t")
						}
					}
					sb.WriteString("\n")
				}
			}
		}
		return &readWordOutput{Path: target, Content: sb.String()}, nil
	}
	return toolutils.InferTool("read_word", "读取 Word (.docx) 文件，返回纯文本内容", fn)
}

type writeWordInput struct {
	Path    string `json:"path"`
	Content string `json:"content"` // 每行作为一个段落
}
type writeWordOutput struct {
	Path string `json:"path"`
}

func writeWordTool(workspaceRoot string) (tool.BaseTool, error) {
	fn := func(_ context.Context, in writeWordInput) (*writeWordOutput, error) {
		target, err := resolveSafePath(workspaceRoot, in.Path)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		d := docx.New().WithA4Page()
		for _, line := range strings.Split(in.Content, "\n") {
			d.AddParagraph().AddText(line)
		}
		f, err := os.Create(target)
		if err != nil {
			return nil, fmt.Errorf("create file: %w", err)
		}
		defer f.Close()
		if _, err := d.WriteTo(f); err != nil {
			return nil, fmt.Errorf("write docx: %w", err)
		}
		return &writeWordOutput{Path: target}, nil
	}
	return toolutils.InferTool("write_word", "生成 Word (.docx) 文件，content 每行作为一个段落", fn)
}

// ==================== Excel ====================

type readExcelInput struct {
	Path  string `json:"path"`
	Sheet string `json:"sheet,omitempty"` // 留空则读第一个 sheet
}
type readExcelOutput struct {
	Path   string     `json:"path"`
	Sheet  string     `json:"sheet"`
	Rows   [][]string `json:"rows"`
	Sheets []string   `json:"sheets"`
}

func readExcelTool(workspaceRoot string) (tool.BaseTool, error) {
	fn := func(_ context.Context, in readExcelInput) (*readExcelOutput, error) {
		target, err := resolveSafePath(workspaceRoot, in.Path)
		if err != nil {
			return nil, err
		}
		f, err := excelize.OpenFile(target)
		if err != nil {
			return nil, fmt.Errorf("open xlsx: %w", err)
		}
		defer f.Close()
		sheets := f.GetSheetList()
		sheetName := in.Sheet
		if sheetName == "" && len(sheets) > 0 {
			sheetName = sheets[0]
		}
		rows, err := f.GetRows(sheetName)
		if err != nil {
			return nil, fmt.Errorf("get rows: %w", err)
		}
		return &readExcelOutput{Path: target, Sheet: sheetName, Rows: rows, Sheets: sheets}, nil
	}
	return toolutils.InferTool("read_excel", "读取 Excel (.xlsx) 文件，返回指定 sheet 的行列数据", fn)
}

type writeExcelInput struct {
	Path  string     `json:"path"`
	Sheet string     `json:"sheet,omitempty"` // 默认 Sheet1
	Rows  [][]string `json:"rows"`            // 二维数组，行×列
}
type writeExcelOutput struct {
	Path  string `json:"path"`
	Sheet string `json:"sheet"`
	Rows  int    `json:"rows"`
}

func writeExcelTool(workspaceRoot string) (tool.BaseTool, error) {
	fn := func(_ context.Context, in writeExcelInput) (*writeExcelOutput, error) {
		target, err := resolveSafePath(workspaceRoot, in.Path)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		sheetName := in.Sheet
		if sheetName == "" {
			sheetName = "Sheet1"
		}
		var xf *excelize.File
		if _, statErr := os.Stat(target); statErr == nil {
			xf, err = excelize.OpenFile(target)
			if err != nil {
				return nil, fmt.Errorf("open xlsx: %w", err)
			}
		} else {
			xf = excelize.NewFile()
			xf.SetSheetName("Sheet1", sheetName)
		}
		defer xf.Close()
		if idx, _ := xf.GetSheetIndex(sheetName); idx == -1 {
			xf.NewSheet(sheetName)
		}
		for r, row := range in.Rows {
			for c, cell := range row {
				colName, _ := excelize.ColumnNumberToName(c + 1)
				xf.SetCellValue(sheetName, fmt.Sprintf("%s%d", colName, r+1), cell)
			}
		}
		if err := xf.SaveAs(target); err != nil {
			return nil, fmt.Errorf("save xlsx: %w", err)
		}
		return &writeExcelOutput{Path: target, Sheet: sheetName, Rows: len(in.Rows)}, nil
	}
	return toolutils.InferTool("write_excel", "生成或更新 Excel (.xlsx) 文件，rows 为二维字符串数组（行×列）", fn)
}

// ==================== PDF ====================

type readPDFInput struct {
	Path      string `json:"path"`
	MaxLength int    `json:"max_length,omitempty"` // 默认 20000
}
type readPDFOutput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
	Pages   int    `json:"pages"`
}

func readPDFTool(workspaceRoot string) (tool.BaseTool, error) {
	fn := func(_ context.Context, in readPDFInput) (*readPDFOutput, error) {
		target, err := resolveSafePath(workspaceRoot, in.Path)
		if err != nil {
			return nil, err
		}
		maxLen := in.MaxLength
		if maxLen <= 0 {
			maxLen = 20000
		}
		f, r, err := pdf.Open(target)
		if err != nil {
			return nil, fmt.Errorf("open pdf: %w", err)
		}
		defer f.Close()
		pages := r.NumPage()
		var sb strings.Builder
		for i := 1; i <= pages; i++ {
			p := r.Page(i)
			if p.V.IsNull() {
				continue
			}
			text, err := p.GetPlainText(nil)
			if err != nil {
				continue
			}
			sb.WriteString(text)
			if sb.Len() >= maxLen {
				break
			}
		}
		content := sb.String()
		if runes := []rune(content); len(runes) > maxLen {
			content = string(runes[:maxLen]) + "\n...[内容已截断]"
		}
		return &readPDFOutput{Path: target, Content: content, Pages: pages}, nil
	}
	return toolutils.InferTool("read_pdf", "读取 PDF 文件，提取纯文本内容", fn)
}

type writePDFInput struct {
	Path    string `json:"path"`
	Title   string `json:"title,omitempty"`
	Content string `json:"content"` // 纯文本，每行作为一段
}
type writePDFOutput struct {
	Path string `json:"path"`
}

func writePDFTool(workspaceRoot string) (tool.BaseTool, error) {
	fn := func(_ context.Context, in writePDFInput) (*writePDFOutput, error) {
		target, err := resolveSafePath(workspaceRoot, in.Path)
		if err != nil {
			return nil, err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return nil, err
		}
		f := gofpdf.New("P", "mm", "A4", "")
		f.AddPage()
		if in.Title != "" {
			f.SetFont("Helvetica", "B", 16)
			f.CellFormat(0, 10, in.Title, "", 1, "C", false, 0, "")
			f.Ln(4)
		}
		f.SetFont("Helvetica", "", 11)
		for _, line := range strings.Split(in.Content, "\n") {
			f.MultiCell(0, 6, line, "", "L", false)
		}
		if err := f.OutputFileAndClose(target); err != nil {
			return nil, fmt.Errorf("write pdf: %w", err)
		}
		return &writePDFOutput{Path: target}, nil
	}
	return toolutils.InferTool("write_pdf", "生成 PDF 文件（内置字体仅支持 Latin，中文内容建议用 write_word）", fn)
}

// ==================== 注册入口 ====================

func buildOfficeTools(workspaceRoot string) ([]tool.BaseTool, error) {
	builders := []func(string) (tool.BaseTool, error){
		readWordTool,
		writeWordTool,
		readExcelTool,
		writeExcelTool,
		readPDFTool,
		writePDFTool,
	}
	result := make([]tool.BaseTool, 0, len(builders))
	for _, b := range builders {
		t, err := b(workspaceRoot)
		if err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, nil
}
