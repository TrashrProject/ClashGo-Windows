package game

import (
	"image"
	"time"

	"github.com/Ducky705/ClashGO/internal/vision"
	"github.com/rs/zerolog"
	"gocv.io/x/gocv"
)

type VillageResourceSnapshot struct {
	Timestamp   time.Time `json:"timestamp"`
	Gold        int       `json:"gold"`
	Elixir      int       `json:"elixir"`
	DarkElixir  int       `json:"dark_elixir"`
	GoldValid   bool      `json:"gold_valid"`
	ElixirValid bool      `json:"elixir_valid"`
	DarkValid   bool      `json:"dark_valid"`
	Valid       bool      `json:"valid"`
}

type VillageResourceReader struct {
	cal       *Calibration
	templates *TemplateStore
	ocr       *LootRecognizer
	logger    zerolog.Logger
}

func NewVillageResourceReader(cal *Calibration, templates *TemplateStore, logger zerolog.Logger) *VillageResourceReader {
	return &VillageResourceReader{
		cal:       cal,
		templates: templates,
		ocr:       NewLootRecognizer(cal, templates, logger),
		logger:    logger.With().Str("component", "village_resources").Logger(),
	}
}

func (r *VillageResourceReader) Close() {
	if r != nil && r.ocr != nil {
		r.ocr.Close()
	}
}

type resourceRead struct {
	value int
	ok    bool
	conf  float64
	icon  image.Point
}

func (r *VillageResourceReader) Read(screen gocv.Mat) VillageResourceSnapshot {
	s := VillageResourceSnapshot{Timestamp: time.Now()}
	if r == nil || r.cal == nil || r.templates == nil || screen.Empty() {
		return s
	}

	gold := r.readOne(screen, "icon_gold")
	elixir := r.readOne(screen, "icon_elixir")
	dark := r.readOne(screen, "icon_de")

	s.Gold, s.GoldValid = gold.value, gold.ok
	s.Elixir, s.ElixirValid = elixir.value, elixir.ok
	s.DarkElixir, s.DarkValid = dark.value, dark.ok

	validCount := 0
	if s.GoldValid { validCount++ }
	if s.ElixirValid { validCount++ }
	if s.DarkValid { validCount++ }
	s.Valid = validCount >= 2

	if s.Valid {
		r.logger.Info().
			Int("gold", s.Gold).
			Int("elixir", s.Elixir).
			Int("dark_elixir", s.DarkElixir).
			Bool("dark_valid", s.DarkValid).
			Msg("village resources snapshot")
	}
	return s
}

func (r *VillageResourceReader) readOne(screen gocv.Mat, templateName string) resourceRead {
	tpl, ok := r.templates.Get(templateName)
	if !ok || tpl.Empty() {
		return resourceRead{}
	}

	w, h := screen.Cols(), screen.Rows()
	search := image.Rect(int(float64(w)*0.52), 0, w, int(float64(h)*0.42))
	matches, err := vision.MatchMultiScaleROICached(
		screen, tpl, templateName,
		0.70, 1.35, 4, 0.62, search,
	)
	if err != nil || len(matches) == 0 {
		return resourceRead{}
	}

	m := matches[0]
	iconW := tpl.Cols()
	if iconW < 20 { iconW = 20 }

	left := image.Rect(
		m.Point.X-int(185*r.cal.ScaleX),
		m.Point.Y-int(22*r.cal.ScaleY),
		m.Point.X-int(float64(iconW)*0.30),
		m.Point.Y+int(22*r.cal.ScaleY),
	)
	right := image.Rect(
		m.Point.X+int(float64(iconW)*0.30),
		m.Point.Y-int(22*r.cal.ScaleY),
		m.Point.X+int(185*r.cal.ScaleX),
		m.Point.Y+int(22*r.cal.ScaleY),
	)

	lv := r.ocr.ReadNumberROI(screen, left)
	rv := r.ocr.ReadNumberROI(screen, right)

	value := lv
	if value <= 0 && rv > 0 {
		value = rv
	}
	if lv > 0 && rv > lv {
		value = rv
	}

	if value <= 0 {
		r.logger.Debug().
			Str("resource", templateName).
			Float64("icon_conf", m.Confidence).
			Int("icon_x", m.Point.X).
			Int("icon_y", m.Point.Y).
			Msg("resource icon found but amount OCR was empty")
		return resourceRead{conf: m.Confidence, icon: m.Point}
	}

	return resourceRead{value: value, ok: true, conf: m.Confidence, icon: m.Point}
}
