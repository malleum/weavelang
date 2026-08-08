-- Buffer options for Weave source.
--
-- `commentstring` is what `gcc` and every other comment mapping reads, so
-- without it nothing knows how to comment a line out. Weave has one comment
-- form, `#` to end of line, and no block comment.
vim.bo.commentstring = "# %s"
vim.bo.comments = ":#"

-- The layout rule counts columns, so a tab would mean one thing to the editor
-- and another to the compiler. `weave fmt` indents with two spaces.
vim.bo.expandtab = true
vim.bo.shiftwidth = 2
vim.bo.tabstop = 2
vim.bo.softtabstop = 2
