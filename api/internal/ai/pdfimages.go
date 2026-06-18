package ai

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
)

const maxScannedPages = 5

// pdfPageImage es una imagen embebida extraída de una página del PDF.
type pdfPageImage struct {
	bytes []byte
	mime  string
}

// pdfImages extrae, por cada una de las primeras maxPages páginas, la imagen
// embebida más grande, solo si es JPEG o PNG (lo que acepta la visión). Go puro
// (pdfcpu). Recupera de panics de la librería. Lista vacía si no hay imágenes
// usables; error si el PDF es inválido/ilegible.
func pdfImages(data []byte, maxPages int) (out []pdfPageImage, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("pdf ilegible: %v", r)
		}
	}()

	// Extracción en memoria de todas las imágenes. ExtractImagesRaw devuelve un
	// slice con una entrada por página (en orden), cada una un map de las imágenes
	// de esa página (clave = objNr/pageNr de la librería). Cada model.Image embebe
	// un io.Reader y trae FileType. Recorremos por índice de slice (= página) y nos
	// quedamos con la imagen JPEG/PNG más grande de cada página.
	pageMaps, xerr := api.ExtractImagesRaw(bytes.NewReader(data), nil, nil)
	if xerr != nil {
		return nil, fmt.Errorf("no pude leer imágenes del pdf: %w", xerr)
	}

	for _, pm := range pageMaps {
		if len(out) >= maxPages {
			break
		}
		// Recorrer en orden de clave para que el "más grande" sea determinista.
		keys := make([]int, 0, len(pm))
		for k := range pm {
			keys = append(keys, k)
		}
		sort.Ints(keys)

		var bestB []byte
		var bestMime string
		for _, k := range keys {
			im := pm[k]
			mime := imageMimeFromType(im.FileType)
			if mime == "" {
				continue // formato no soportado por la visión (tif/ccitt/jbig2…)
			}
			b, rerr := io.ReadAll(im)
			if rerr != nil || len(b) == 0 {
				continue
			}
			if len(b) > len(bestB) {
				bestB = b
				bestMime = mime
			}
		}
		if len(bestB) > 0 {
			out = append(out, pdfPageImage{bytes: bestB, mime: bestMime})
		}
	}
	return out, nil
}

// imageMimeFromType mapea el tipo de imagen de pdfcpu al mime que acepta la visión.
func imageMimeFromType(ft string) string {
	switch strings.ToLower(strings.TrimPrefix(ft, ".")) {
	case "jpg", "jpeg":
		return "image/jpeg"
	case "png":
		return "image/png"
	}
	return ""
}
