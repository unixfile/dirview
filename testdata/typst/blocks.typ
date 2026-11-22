#heading(level: 2)[Lists]#label("lists")

#list(tight: true,
[one],
[two
#list(tight: true,
[nested],
)],
[☐ todo],
[☑ done],
)

#enum(tight: true, start: 3,
[three],
[four],
)

#list(tight: false,
[loose],
[list],
)

#quote(block: true)[A quote
over lines\.]

#quote(block: true)[#text(font: "Iosevka", size: 0.8em)[note]

Read this\.]

#raw(block: true, "func main() {\n\tfmt.Println(\"a \\\"quoted\\\" \\\\ line\")\n}")

#raw(block: true, "indented code")

#line(length: 100%)

#terms(tight: true,
terms.item[Term][Its definition\.],
terms.item[Other][First\.

Second\.],
)

#align(center, table(columns: 4, align: (left, right, center, auto,),
table.header([left], [right], [centre], [none],),
[1], [2], [], [],
[a \| b], [#raw("c")], [#strong[d]], [e],
))

Line\
break and kept text\.

A note#footnote[The note\.]#label("fn:1") and again#footnote(label("fn:1"))\, and another#footnote[First paragraph\.

Second paragraph\.]#label("fn:2")\.

#align(center, table(columns: 1, align: (auto,),
table.header([one],),
[column],
))
