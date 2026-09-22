package clone

// Ces alias ouvrent aux tests la recherche du dossier des téléchargements,
// pour l'éprouver sur les trois systèmes depuis n'importe quelle machine.

type System = system

var (
	DefaultParentOn = defaultParent
	KnownDownloads  = knownDownloads
)
