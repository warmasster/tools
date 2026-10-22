package main

// Tipos de archivo que se usan para colorear el mapa y el resumen por tipo.
type Category struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Color string `json:"color"`
}

var categories = []Category{
	{"otros", "Otros", "#5b6478"},
	{"video", "Vídeo", "#8b5cf6"},
	{"imagen", "Imágenes", "#22d3ee"},
	{"audio", "Música y audio", "#fb923c"},
	{"juegos", "Datos de juegos", "#fbbf24"},
	{"comprimidos", "Comprimidos e imágenes de disco", "#f87171"},
	{"programas", "Programas", "#6366f1"},
	{"documentos", "Documentos", "#34d399"},
	{"codigo", "Código", "#a3e635"},
	{"ia", "Modelos de IA", "#e879f9"},
	{"sistema", "Temporales, cachés y registros", "#94a3b8"},
}

var extCategory = map[string]uint8{}

func init() {
	add := func(cat uint8, exts ...string) {
		for _, e := range exts {
			extCategory[e] = cat
		}
	}
	add(1, "mp4", "mkv", "avi", "mov", "wmv", "webm", "m4v", "flv", "mpg", "mpeg", "ts", "m2ts", "mts", "3gp", "vob", "ogv")
	add(2, "jpg", "jpeg", "png", "gif", "bmp", "webp", "heic", "heif", "tif", "tiff", "raw", "cr2", "cr3", "nef", "arw", "dng", "psd", "svg", "ico", "avif", "jxl", "exr", "hdr", "dds", "tga", "kra", "xcf")
	add(3, "mp3", "flac", "wav", "aac", "ogg", "m4a", "wma", "opus", "aiff", "alac", "mid", "midi", "wem", "bnk", "fsb")
	add(4, "pak", "vpk", "bsa", "ba2", "esm", "esp", "forge", "ucas", "utoc", "uasset", "umap", "assets", "bundle", "resource", "ress", "wad", "big", "rpf", "gcf", "ncf", "acf", "bk2", "bik", "upk", "xxx", "arc", "dat001", "pck", "sav")
	add(5, "zip", "rar", "7z", "tar", "gz", "tgz", "bz2", "xz", "zst", "lz4", "cab", "iso", "img", "vhd", "vhdx", "vmdk", "vdi", "qcow2", "wim", "esd", "dmg", "apk", "xapk", "nupkg", "whl", "jar", "war")
	add(6, "exe", "dll", "sys", "msi", "msix", "msixbundle", "appx", "appxbundle", "msp", "ocx", "drv", "efi", "mui", "node", "so", "dylib", "bin", "com", "scr", "cpl")
	add(7, "pdf", "doc", "docx", "xls", "xlsx", "xlsm", "ppt", "pptx", "odt", "ods", "odp", "rtf", "txt", "md", "epub", "mobi", "azw3", "csv", "pages", "numbers", "key", "one", "pst", "ost", "eml", "msg")
	add(8, "js", "mjs", "cjs", "tsx", "jsx", "go", "py", "pyc", "pyd", "c", "h", "cpp", "hpp", "cc", "cs", "java", "class", "kt", "rs", "rb", "php", "swift", "lua", "json", "yaml", "yml", "toml", "xml", "html", "htm", "css", "scss", "map", "sql", "sh", "ps1", "bat", "cmd", "pdb", "lib", "obj", "o", "a", "ilk", "ipch", "wasm", "lock", "vue", "svelte")
	add(9, "gguf", "ggml", "safetensors", "ckpt", "pt", "pth", "onnx", "tflite", "h5", "keras", "pb", "mlmodel", "engine", "npz", "npy")
	add(10, "tmp", "temp", "log", "etl", "dmp", "mdmp", "hdmp", "cache", "old", "bak", "chk", "crdownload", "part", "partial", "download", "evtx", "blf", "regtrans-ms", "db-wal", "db-shm", "ldb", "thumbs", "edb", "jrs")
}

func categoryOf(ext string) uint8 {
	return extCategory[ext]
}
