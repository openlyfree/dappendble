# Dappendble 


that one time i accidentally reinvented bitcask

i thought i genuinely made the worlds fastest embedded rdbms for a sec (by a sec i mean 4 months)

but its just log structured storage engine

that i some how made support sql (kinda)

yeah im gonna need a while to comtemplate my life choices

It is a Go log-structured cell store (Add/At/Delete on (col, row)), mmap files plus a RAM index, with a database/sql driver that speaks a tiny SQL subset. Not an RDBMS. Not faster Bitcask. Crash can eat the last second of writes.

<img width="277" height="250" alt="cat-crying" src="https://github.com/user-attachments/assets/84db4062-d3f8-4f18-af5b-56ad860e7803" />
