#heading(level: 1)[A #link("https://example.com")[linked] #raw("code") heading]#label("a-linkedhttpsexamplecom-code-heading")

#heading(level: 1)[]#label("heading")

Nested #emph[#strong[strong in emphasis]] and #emph[emphasis #strong[with strong] inside]\, a #link("https://example.com/empty")[] link and #raw("a `tick` and \"quote\" \\ in code")\.

A note that names itself#footnote[It says#footnote(label("fn:1")) once more\.]#label("fn:1")\.

#quote(block: true)[Outer

#quote(block: true)[Inner with a list\:

#list(tight: true,
[item],
)

#raw(block: true, "code in a quote")]]

#quote(block: true)[#text(font: "Iosevka", size: 0.8em)[tip]]

#quote(block: true)[\[\!FOO\]
Not an alert\.]

#list(tight: false,
[☐ loose task],
[☑ another],
)

#align(center, table(columns: 2, align: (auto, auto,),
table.header([#raw("a|b")], [x],),
[], [trailing \\],
))

#terms(tight: true,
terms.item[First][],
terms.item[Second][Shared description\.],
)

A paragraph that ends in a backslash\\
