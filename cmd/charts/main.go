// Command charts renders the benchmark results to standalone SVG files for the
// blog post. Pure standard library: the SVGs are built as plain strings. Run:
//
//	go run ./cmd/charts        # writes charts/*.svg
//
// Data is the GOMAXPROCS=1/2/4/8 latency (ns/op) from results/by-impl.txt.
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
)

var impls = []string{"mutex", "rwmutex", "syncmap", "sharded", "cow"}
var colors = []string{"#5F5E5A", "#378ADD", "#BA7517", "#1D9E75", "#D85A30"}
var dashes = []string{"", "6,4", "2,3", "", "8,4,2,4"}
var widths = []float64{2, 2, 2, 3, 2}

var cores = []float64{1, 2, 4, 8}

type quad = [4]float64

// ns[dist][mix] holds one quad per implementation (same order as `impls`).
// Pinned to physical P-cores (affinity 0x5555); see cmd/cpuinfo and
// INMEMCACHE_AFFINITY in affinity_windows_test.go.
var ns = map[string]map[string][5]quad{
	"uniform": {
		"r100": {{111.3, 147.45, 157.25, 168.20}, {110.7, 62.51, 53.05, 52.67}, {252.9, 120.70, 59.46, 30.14}, {144.9, 71.98, 37.67, 21.15}, {105.4, 46.60, 22.78, 11.50}},
		"r90":  {{112.4, 154.00, 161.55, 168.45}, {117.0, 215.80, 238.75, 258.70}, {294.5, 141.30, 69.70, 36.66}, {145.1, 74.76, 39.20, 22.12}, {17381385, 6091222, 10090020, 11977900}},
		"r50":  {{124.9, 174.40, 183.55, 189.65}, {136.8, 222.50, 266.75, 282.15}, {405.2, 232.50, 106.90, 56.87}, {146.7, 77.36, 41.12, 23.55}, {70848057, 47800015, 48217095, 46490005}},
		"r10":  {{137.7, 192.25, 199.75, 207.50}, {144.1, 223.05, 220.95, 222.40}, {675.7, 324.40, 139.65, 73.25}, {147.2, 79.98, 43.33, 24.95}, {128343200, 98046427, 92461767, 82537162}},
	},
	"zipf": {
		"r100": {{74.28, 80.88, 92.21, 106.20}, {73.03, 58.29, 50.07, 49.085}, {144.80, 66.71, 32.57, 15.96}, {97.52, 49.90, 28.12, 17.44}, {69.60, 31.36, 15.16, 7.347}},
		"r90":  {{74.74, 85.90, 101.75, 112.05}, {75.43, 185.00, 212.05, 224.80}, {166.75, 76.87, 41.68, 23.55}, {100.15, 57.16, 34.79, 23.55}, {15612972, 5613417, 7314545, 9038677}},
		"r50":  {{76.89, 93.86, 115.95, 125.85}, {80.11, 150.05, 174.75, 182.65}, {282.80, 145.50, 78.89, 45.61}, {102.15, 61.99, 40.28, 28.68}, {77380765, 51467177, 45527752, 45103177}},
		"r10":  {{79.27, 96.66, 120.00, 131.20}, {95.87, 117.10, 135.05, 141.65}, {431.60, 195.70, 108.05, 67.86}, {101.30, 65.35, 43.93, 32.04}, {120233780, 88179842, 82798450, 84026612}},
	},
}

var mixTitles = map[string]string{
	"r100": "read-only (r100)", "r90": "read-heavy (r90)",
	"r50": "balanced (r50)", "r10": "write-heavy (r10)",
}

const txt = "#2C2C2A"  // near-black, for light backgrounds
const muted = "#5F5E5A"
const gridc = "rgba(120,120,120,0.18)"

func tput(nsv float64) float64 { return 1000.0 / nsv } // Mops/s

func f(b *strings.Builder, format string, a ...any) { fmt.Fprintf(b, format, a...) }

// linePanel draws a throughput-vs-cores panel at (px,py) sized pw x ph.
func linePanel(b *strings.Builder, px, py, pw, ph float64, title string, data [5]quad, ymax float64) {
	const ml, mr, mt, mb = 50, 16, 26, 34
	plotW := pw - ml - mr
	plotH := ph - mt - mb
	x0, y0 := px+ml, py+mt
	sx := func(i int) float64 { return x0 + float64(i)*plotW/3 }
	sy := func(v float64) float64 { return y0 + plotH*(1-v/ymax) }

	f(b, `<text x="%.1f" y="%.1f" font-size="13" font-weight="500" fill="%s">%s</text>`, px+ml, py+14, txt, title)
	// y gridlines + labels
	for k := 0; k <= 4; k++ {
		v := ymax * float64(k) / 4
		yy := sy(v)
		f(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, x0, yy, x0+plotW, yy, gridc)
		f(b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="end">%.0f</text>`, x0-6, yy+3, muted, v)
	}
	// x labels
	for i := range cores {
		f(b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="middle">%.0f</text>`, sx(i), y0+plotH+16, muted, cores[i])
	}
	f(b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="middle">GOMAXPROCS</text>`, x0+plotW/2, y0+plotH+30, muted)
	f(b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="middle" transform="rotate(-90 %.1f %.1f)">Mops/s</text>`, px+12, y0+plotH/2, muted, px+12, y0+plotH/2)

	// series
	for s := range impls {
		var pts strings.Builder
		for i := 0; i < 4; i++ {
			pts.WriteString(fmt.Sprintf("%.1f,%.1f ", sx(i), sy(tput(data[s][i]))))
		}
		dash := ""
		if dashes[s] != "" {
			dash = fmt.Sprintf(` stroke-dasharray="%s"`, dashes[s])
		}
		f(b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%.1f"%s/>`, strings.TrimSpace(pts.String()), colors[s], widths[s], dash)
		for i := 0; i < 4; i++ {
			f(b, `<circle cx="%.1f" cy="%.1f" r="2.6" fill="%s"/>`, sx(i), sy(tput(data[s][i])), colors[s])
		}
	}
}

func legend(b *strings.Builder, x, y float64) {
	cx := x
	for s := range impls {
		dash := ""
		if dashes[s] != "" {
			dash = fmt.Sprintf(` stroke-dasharray="%s"`, dashes[s])
		}
		f(b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="3"%s/>`, cx, y, cx+22, y, colors[s], dash)
		f(b, `<text x="%.1f" y="%.1f" font-size="12" fill="%s">%s</text>`, cx+27, y+4, txt, impls[s])
		cx += 27 + float64(len(impls[s]))*7.5 + 20
	}
}

func svgHeader(b *strings.Builder, w, h float64, title, desc string) {
	f(b, `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %.0f %.0f" width="%.0f" height="%.0f" font-family="-apple-system,Segoe UI,Roboto,sans-serif" role="img">`, w, h, w, h)
	f(b, `<title>%s</title><desc>%s</desc>`, title, desc)
}

func writeFile(name, content string) {
	path := filepath.Join("charts", name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Println("wrote", path)
}

// gridChart: 2x2 small multiples of throughput-vs-cores, one per mix.
func gridChart(dist string, ymax float64) {
	const w, h = 760, 540
	var b strings.Builder
	svgHeader(&b, w, h, "Throughput vs cores by mix ("+dist+")",
		"Throughput in millions of ops/sec versus GOMAXPROCS for five cache implementations, "+dist+" key distribution, one panel per read/write mix.")
	f(&b, `<text x="20" y="26" font-size="15" font-weight="500" fill="%s">throughput vs cores by mix &#8212; %s distribution (1M keys)</text>`, txt, dist)
	legend(&b, 20, 48)
	order := []string{"r100", "r90", "r50", "r10"}
	pw, ph := (w-30)/2.0, (h-70)/2.0
	for idx, mix := range order {
		col := float64(idx % 2)
		row := float64(idx / 2)
		px := 10 + col*(pw+10)
		py := 64 + row*(ph+6)
		linePanel(&b, px, py, pw, ph, mixTitles[mix], ns[dist][mix], ymax)
	}
	b.WriteString("</svg>")
	writeFile("throughput_by_mix_"+dist+".svg", b.String())
}

// efficiencyChart: speedup vs 1-core for read-only/uniform, with ideal line.
func efficiencyChart() {
	const w, h = 720, 460
	const ml, mr, mt, mb = 60, 20, 70, 60
	plotW, plotH := float64(w-ml-mr), float64(h-mt-mb)
	x0, y0 := float64(ml), float64(mt)
	ymax := 9.0
	sx := func(i int) float64 { return x0 + float64(i)*plotW/3 }
	sy := func(v float64) float64 { return y0 + plotH*(1-v/ymax) }
	base := ns["uniform"]["r100"]

	var b strings.Builder
	svgHeader(&b, w, h, "Scaling efficiency (read-only, uniform)",
		"Throughput speedup relative to a single core versus GOMAXPROCS, read-only uniform workload, with an ideal linear-scaling reference.")
	f(&b, `<text x="20" y="26" font-size="15" font-weight="500" fill="%s">scaling efficiency &#8212; read-only, uniform</text>`, txt)
	f(&b, `<text x="20" y="44" font-size="12" fill="%s">speedup vs 1 core; dotted line = perfect linear scaling</text>`, muted)
	legend(&b, 20, 60)

	for k := 0; k <= 9; k++ {
		yy := sy(float64(k))
		f(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, x0, yy, x0+plotW, yy, gridc)
		f(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="end">%d&#215;</text>`, x0-6, yy+3, muted, k)
	}
	for i := range cores {
		f(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s" text-anchor="middle">%.0f</text>`, sx(i), y0+plotH+18, muted, cores[i])
	}
	f(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s" text-anchor="middle">GOMAXPROCS</text>`, x0+plotW/2, y0+plotH+38, muted)
	f(&b, `<text x="22" y="%.1f" font-size="11" fill="%s" text-anchor="middle" transform="rotate(-90 22 %.1f)">speedup vs 1 core</text>`, y0+plotH/2, muted, y0+plotH/2)

	// ideal line (1,2,4,8)
	var ideal strings.Builder
	for i := 0; i < 4; i++ {
		ideal.WriteString(fmt.Sprintf("%.1f,%.1f ", sx(i), sy(cores[i])))
	}
	f(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="1.5" stroke-dasharray="2,3"/>`, strings.TrimSpace(ideal.String()), muted)
	f(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="end">ideal 8&#215;</text>`, sx(3)-4, sy(8)-4, muted)

	for s := range impls {
		var pts strings.Builder
		for i := 0; i < 4; i++ {
			pts.WriteString(fmt.Sprintf("%.1f,%.1f ", sx(i), sy(base[s][0]/base[s][i])))
		}
		dash := ""
		if dashes[s] != "" {
			dash = fmt.Sprintf(` stroke-dasharray="%s"`, dashes[s])
		}
		f(&b, `<polyline points="%s" fill="none" stroke="%s" stroke-width="%.1f"%s/>`, strings.TrimSpace(pts.String()), colors[s], widths[s], dash)
		for i := 0; i < 4; i++ {
			f(&b, `<circle cx="%.1f" cy="%.1f" r="3" fill="%s"/>`, sx(i), sy(base[s][0]/base[s][i]), colors[s])
		}
	}
	b.WriteString("</svg>")
	writeFile("scaling_efficiency_r100_uniform.svg", b.String())
}

// skewChart: bar chart of skew speedup (uniform/zipf latency) at 8 cores.
func skewChart() {
	const w, h = 720, 440
	const ml, mr, mt, mb = 54, 16, 76, 50
	plotW, plotH := float64(w-ml-mr), float64(h-mt-mb)
	x0, y0 := float64(ml), float64(mt)
	ymax := 2.0
	sy := func(v float64) float64 { return y0 + plotH*(1-v/ymax) }
	mixes := []string{"r100", "r50"}
	barColors := map[string]string{"r100": "#1D9E75", "r50": "#BA7517"}
	mixLabel := map[string]string{"r100": "read-only", "r50": "balanced"}

	var b strings.Builder
	svgHeader(&b, w, h, "Skew speedup at 8 cores",
		"Latency ratio of uniform to Zipfian access at 8 cores per implementation; above 1 means skew helps, below 1 means skew hurts.")
	f(&b, `<text x="20" y="26" font-size="15" font-weight="500" fill="%s">effect of skew &#8212; uniform &#247; zipf latency at 8 cores</text>`, txt)
	f(&b, `<text x="20" y="44" font-size="12" fill="%s">above 1.0&#215; = faster under skew (cache locality); below = slower (hot-key contention)</text>`, muted)
	// legend for the two mixes
	cx := 20.0
	for _, mix := range mixes {
		f(&b, `<rect x="%.1f" y="54" width="11" height="11" rx="2" fill="%s"/>`, cx, barColors[mix])
		f(&b, `<text x="%.1f" y="63" font-size="12" fill="%s">%s</text>`, cx+16, txt, mixLabel[mix])
		cx += 16 + float64(len(mixLabel[mix]))*7.5 + 22
	}

	for k := 0; k <= 4; k++ {
		v := ymax * float64(k) / 4
		yy := sy(v)
		f(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, x0, yy, x0+plotW, yy, gridc)
		f(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="end">%.1f&#215;</text>`, x0-6, yy+3, muted, v)
	}
	// reference line at 1.0
	f(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1.5" stroke-dasharray="5,3"/>`, x0, sy(1), x0+plotW, sy(1), "#A32D2D")

	groupW := plotW / float64(len(impls))
	bw := groupW * 0.28
	for s := range impls {
		gx := x0 + float64(s)*groupW + groupW/2
		f(&b, `<text x="%.1f" y="%.1f" font-size="12" fill="%s" text-anchor="middle">%s</text>`, gx, y0+plotH+20, txt, impls[s])
		for mi, mix := range mixes {
			ratio := ns["uniform"][mix][s][3] / ns["zipf"][mix][s][3]
			bx := gx + (float64(mi)-0.5)*(bw+4) - bw/2 + (bw+4)/2
			by := sy(ratio)
			top := math.Min(by, sy(0))
			height := math.Abs(sy(0) - by)
			f(&b, `<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="2" fill="%s"/>`, bx-bw/2, top, bw, height, barColors[mix])
			f(&b, `<text x="%.1f" y="%.1f" font-size="9.5" fill="%s" text-anchor="middle">%.2f</text>`, bx, by-4, muted, ratio)
		}
	}
	b.WriteString("</svg>")
	writeFile("skew_speedup_8cores.svg", b.String())
}

// shardCountChart: throughput vs shard count (balanced mix, uniform, 8 cores),
// showing where the contention curve flattens. Answers "why 256?".
func shardCountChart() {
	const w, h = 720, 430
	const ml, mr, mt, mb = 58, 20, 66, 54
	plotW, plotH := float64(w-ml-mr), float64(h-mt-mb)
	x0, y0 := float64(ml), float64(mt)
	labels := []string{"1", "16", "64", "256", "1024", "4096"}
	nsv := []float64{215.5, 46.76, 29.91, 23.45, 20.75, 19.85}
	ymax := 55.0
	sx := func(i int) float64 { return x0 + float64(i)*plotW/float64(len(labels)-1) }
	sy := func(v float64) float64 { return y0 + plotH*(1-v/ymax) }

	var b strings.Builder
	svgHeader(&b, w, h, "Shard count vs throughput",
		"Throughput in millions of ops/sec versus shard count, balanced 50/50 mix, uniform keys, 8 cores; the curve flattens past 256 shards.")
	f(&b, `<text x="20" y="26" font-size="15" font-weight="500" fill="%s">how many shards? &#8212; balanced mix, uniform, 8 cores</text>`, txt)
	f(&b, `<text x="20" y="44" font-size="12" fill="%s">more shards &#8594; less lock contention, with sharp diminishing returns past 256</text>`, muted)

	for k := 0; k <= 5; k++ {
		v := ymax * float64(k) / 5
		yy := sy(v)
		f(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="%s" stroke-width="1"/>`, x0, yy, x0+plotW, yy, gridc)
		f(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="end">%.0f</text>`, x0-6, yy+3, muted, v)
	}
	for i := range labels {
		f(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s" text-anchor="middle">%s</text>`, sx(i), y0+plotH+18, muted, labels[i])
	}
	f(&b, `<text x="%.1f" y="%.1f" font-size="11" fill="%s" text-anchor="middle">shards</text>`, x0+plotW/2, y0+plotH+38, muted)
	f(&b, `<text x="20" y="%.1f" font-size="11" fill="%s" text-anchor="middle" transform="rotate(-90 20 %.1f)">Mops/s</text>`, y0+plotH/2, muted, y0+plotH/2)

	hx := sx(3) // highlight default = 256
	f(&b, `<line x1="%.1f" y1="%.1f" x2="%.1f" y2="%.1f" stroke="#534AB7" stroke-width="1.5" stroke-dasharray="4,3"/>`, hx, y0, hx, y0+plotH)
	f(&b, `<text x="%.1f" y="%.1f" font-size="10.5" fill="#534AB7" text-anchor="middle">default = 256</text>`, hx, y0-5)

	var pts strings.Builder
	for i := range labels {
		pts.WriteString(fmt.Sprintf("%.1f,%.1f ", sx(i), sy(1000.0/nsv[i])))
	}
	f(&b, `<polyline points="%s" fill="none" stroke="#1D9E75" stroke-width="3"/>`, strings.TrimSpace(pts.String()))
	for i := range labels {
		t := 1000.0 / nsv[i]
		f(&b, `<circle cx="%.1f" cy="%.1f" r="3.5" fill="#1D9E75"/>`, sx(i), sy(t))
		f(&b, `<text x="%.1f" y="%.1f" font-size="10" fill="%s" text-anchor="middle">%.0f</text>`, sx(i), sy(t)-8, txt, t)
	}
	b.WriteString("</svg>")
	writeFile("shard_count_8cores.svg", b.String())
}

func main() {
	if err := os.MkdirAll("charts", 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	gridChart("uniform", 92)
	gridChart("zipf", 145)
	efficiencyChart()
	skewChart()
	shardCountChart()
}
