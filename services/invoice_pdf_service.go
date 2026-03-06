package services

import (
	"bytes"
	"fmt"
	"strings"

	"salonpro-backend/models"

	"github.com/jung-kurt/gofpdf"
)

// InvoicePDFData bundles all data needed to render a PDF.
type InvoicePDFData struct {
	Invoice  models.Invoice
	Salon    models.Salon
	Customer models.Customer
}

// BuildInvoicePDF generates a single-page styled invoice PDF in memory and returns the bytes.
// No file is written to disk. Reused by GetInvoicePDF (HTTP) and SendInvoiceEmail.
func BuildInvoicePDF(data InvoicePDFData) ([]byte, error) {
	inv := data.Invoice
	salon := data.Salon
	customer := data.Customer

	// A4 portrait, 15mm side margins, NO auto page-break so footer stays on page 1
	pdf := gofpdf.New("P", "mm", "A4", "")
	pdf.SetMargins(15, 15, 15)
	pdf.SetAutoPageBreak(false, 0)
	pdf.AddPage()

	const (
		pageW  = 180.0 // 210 - 15 - 15
		leftX  = 15.0
		rightX = 195.0
	)

	// ── colour helpers ───────────────────────────────────────────────────────
	setTxt := func(r, g, b int) { pdf.SetTextColor(r, g, b) }
	setFill := func(r, g, b int) { pdf.SetFillColor(r, g, b) }
	setDraw := func(r, g, b int) { pdf.SetDrawColor(r, g, b) }
	dark := func() { setTxt(30, 30, 30) }

	// ── HEADER BAND (height 26mm) ────────────────────────────────────────────
	setFill(25, 25, 35)
	pdf.Rect(0, 0, 210, 26, "F")

	pdf.SetFont("Arial", "B", 18)
	setTxt(255, 255, 255)
	pdf.SetXY(leftX, 5)
	pdf.CellFormat(pageW*0.6, 9, salon.Name, "", 1, "L", false, 0, "")

	pdf.SetFont("Arial", "", 8)
	setTxt(190, 190, 210)
	if salon.Address != "" {
		pdf.SetX(leftX)
		pdf.CellFormat(pageW*0.6, 5, salon.Address, "", 1, "L", false, 0, "")
	}

	pdf.SetFont("Arial", "B", 20)
	setTxt(255, 215, 0)
	pdf.SetXY(210-70, 7)
	pdf.CellFormat(55, 10, "INVOICE", "", 1, "R", false, 0, "")

	// ── META SECTION (two columns, fixed Y per row) ──────────────────────────
	// Build each column as a slice so we can render row by row with fixed Y,
	// guaranteeing both columns stay aligned and don't push each other.
	type metaRow struct{ label, value string }

	leftRows := []metaRow{
		{"INVOICE NO", inv.InvoiceNumber},
		{"DATE", inv.InvoiceDate.Format("02 Jan 2006")},
	}
	rightRows := []metaRow{
		{"CUSTOMER", customer.Name},
	}
	if customer.Phone != "" {
		rightRows = append(rightRows, metaRow{"PHONE", customer.Phone})
	}
	if customer.Email != "" {
		rightRows = append(rightRows, metaRow{"EMAIL", customer.Email})
	}
	pmName := inv.PaymentMethod.Name
	if pmName == "" {
		pmName = "—"
	}
	rightRows = append(rightRows, metaRow{"PAYMENT METHOD", pmName})

	const (
		metaStartY = 30.0
		metaRowH   = 7.0
		labelW     = 40.0
		valW       = 45.0
		col1X      = leftX
		col2X      = 110.0
	)

	renderMetaRow := func(x, y float64, label, value string) {
		pdf.SetXY(x, y)
		pdf.SetFont("Arial", "B", 8)
		setTxt(100, 100, 120)
		pdf.CellFormat(labelW, metaRowH, label, "", 0, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		dark()
		pdf.CellFormat(valW, metaRowH, value, "", 0, "L", false, 0, "")
	}

	for i, r := range leftRows {
		renderMetaRow(col1X, metaStartY+float64(i)*metaRowH, r.label, r.value)
	}
	for i, r := range rightRows {
		renderMetaRow(col2X, metaStartY+float64(i)*metaRowH, r.label, r.value)
	}

	// Status badge (below left column rows)
	badgeY := metaStartY + float64(len(leftRows))*metaRowH
	pdf.SetXY(col1X, badgeY)
	pdf.SetFont("Arial", "B", 8)
	setTxt(100, 100, 120)
	pdf.CellFormat(labelW, metaRowH, "STATUS", "", 0, "L", false, 0, "")
	statusText := strings.ToUpper(inv.PaymentStatus)
	switch strings.ToLower(inv.PaymentStatus) {
	case "paid":
		setFill(34, 197, 94)
	case "partial":
		setFill(234, 179, 8)
	default:
		setFill(239, 68, 68)
	}
	setTxt(255, 255, 255)
	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(28, metaRowH-1, statusText, "0", 0, "C", true, 0, "")
	dark()

	// Y after meta: max of left and right columns
	leftEndY := metaStartY + float64(len(leftRows)+1)*metaRowH // +1 for status badge
	rightEndY := metaStartY + float64(len(rightRows))*metaRowH
	afterMetaY := leftEndY
	if rightEndY > afterMetaY {
		afterMetaY = rightEndY
	}
	afterMetaY += 3

	// ── DIVIDER ──────────────────────────────────────────────────────────────
	setDraw(210, 210, 210)
	pdf.Line(leftX, afterMetaY, rightX, afterMetaY)

	// ── ITEMS TABLE ──────────────────────────────────────────────────────────
	const (
		cSvc      = 87.0
		cQty      = 18.0
		cUP       = 37.0
		cTotal    = 38.0
		tableHdrH = 7.0
		tableRowH = 7.0
	)
	tableStartY := afterMetaY + 3

	// Header row
	pdf.SetXY(leftX, tableStartY)
	setFill(25, 25, 35)
	setTxt(255, 255, 255)
	setDraw(25, 25, 35)
	pdf.SetFont("Arial", "B", 8)
	pdf.CellFormat(cSvc, tableHdrH, "  SERVICE", "0", 0, "L", true, 0, "")
	pdf.CellFormat(cQty, tableHdrH, "QTY", "0", 0, "C", true, 0, "")
	pdf.CellFormat(cUP, tableHdrH, "UNIT PRICE", "0", 0, "R", true, 0, "")
	pdf.CellFormat(cTotal, tableHdrH, "TOTAL  ", "0", 1, "R", true, 0, "")
	setDraw(220, 220, 220)
	dark()

	// Data rows
	pdf.SetFont("Arial", "", 9)
	for i, item := range inv.Items {
		if i%2 == 0 {
			setFill(248, 248, 252)
		} else {
			setFill(255, 255, 255)
		}
		pdf.SetX(leftX)
		pdf.CellFormat(cSvc, tableRowH, "  "+item.ServiceName, "B", 0, "L", true, 0, "")
		pdf.CellFormat(cQty, tableRowH, fmt.Sprintf("%d", item.Quantity), "B", 0, "C", true, 0, "")
		pdf.CellFormat(cUP, tableRowH, fmt.Sprintf("%.2f", item.UnitPrice), "B", 0, "R", true, 0, "")
		pdf.CellFormat(cTotal, tableRowH, fmt.Sprintf("%.2f  ", item.TotalPrice), "B", 1, "R", true, 0, "")
	}

	afterTableY := tableStartY + tableHdrH + float64(len(inv.Items))*tableRowH + 3

	// ── TOTALS BLOCK ─────────────────────────────────────────────────────────
	const (
		totX    = 115.0
		totLblW = 45.0
		totValW = 35.0
		totRowH = 6.5
	)

	totY := afterTableY
	writeTot := func(label, value string, bold, highlight bool) {
		pdf.SetXY(totX, totY)
		if bold {
			pdf.SetFont("Arial", "B", 9)
		} else {
			pdf.SetFont("Arial", "", 9)
		}
		if highlight {
			setFill(25, 25, 35)
			setTxt(255, 255, 255)
			pdf.CellFormat(totLblW, totRowH+0.5, " "+label, "0", 0, "L", true, 0, "")
			pdf.CellFormat(totValW, totRowH+0.5, value+"  ", "0", 0, "R", true, 0, "")
			dark()
		} else {
			setTxt(80, 80, 100)
			pdf.CellFormat(totLblW, totRowH, label, "0", 0, "L", false, 0, "")
			dark()
			pdf.CellFormat(totValW, totRowH, value+"  ", "0", 0, "R", false, 0, "")
		}
		totY += totRowH
	}

	writeTot("Subtotal", fmt.Sprintf("%.2f", inv.Subtotal), false, false)
	if inv.Discount > 0 {
		writeTot("Discount", fmt.Sprintf("-%.2f", inv.Discount), false, false)
	}
	if inv.Tax > 0 {
		writeTot(fmt.Sprintf("Tax (%.0f%%)", inv.Tax), fmt.Sprintf("%.2f", inv.Subtotal*inv.Tax/100), false, false)
	}
	// thin divider line
	setDraw(180, 180, 200)
	pdf.Line(totX, totY, rightX, totY)
	totY += 1
	writeTot("TOTAL", fmt.Sprintf("%.2f", inv.Total), true, true)
	totY += 1
	writeTot("Paid", fmt.Sprintf("%.2f", inv.PaidAmount), false, false)
	if balance := inv.Total - inv.PaidAmount; balance > 0 {
		pdf.SetXY(totX, totY)
		pdf.SetFont("Arial", "B", 9)
		setTxt(220, 60, 60)
		pdf.CellFormat(totLblW, totRowH, "Balance Due", "0", 0, "L", false, 0, "")
		pdf.CellFormat(totValW, totRowH, fmt.Sprintf("%.2f  ", balance), "0", 0, "R", false, 0, "")
		totY += totRowH
		dark()
	}

	// ── NOTES (left side, same vertical band as totals) ──────────────────────
	if inv.Notes != "" {
		notesY := afterTableY
		pdf.SetXY(leftX, notesY)
		pdf.SetFont("Arial", "B", 8)
		setTxt(100, 100, 120)
		pdf.CellFormat(85, 5.5, "NOTES", "", 1, "L", false, 0, "")
		pdf.SetFont("Arial", "", 9)
		dark()
		pdf.SetX(leftX)
		pdf.MultiCell(85, 5, inv.Notes, "", "L", false)
	}

	// ── FOOTER BAND (pinned to bottom of page) ───────────────────────────────
	// Always at fixed Y so it never overflows to page 2
	const footerY = 274.0 // A4 = 297mm; footer band is 23mm tall starting at 274
	setFill(25, 25, 35)
	pdf.Rect(0, footerY, 210, 30, "F")

	pdf.SetFont("Arial", "I", 9)
	setTxt(190, 190, 210)
	pdf.SetXY(leftX, footerY+4)
	pdf.CellFormat(pageW, 6, "Thank you for your business! We look forward to seeing you again.", "", 1, "C", false, 0, "")
	pdf.SetFont("Arial", "", 8)
	setTxt(140, 140, 160)
	pdf.SetX(leftX)
	pdf.CellFormat(pageW, 5, salon.Name+" • "+salon.Address, "", 1, "C", false, 0, "")

	var buf bytes.Buffer
	if err := pdf.Output(&buf); err != nil {
		return nil, fmt.Errorf("failed to render PDF: %w", err)
	}
	return buf.Bytes(), nil
}
