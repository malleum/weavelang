# Weave in nixvim

Everything below assumes the flake is an input of your configuration under the
name `weave`:

```nix
# flake.nix
{
  inputs.weave.url = "github:malleum/weave";
}
```

and that your nixvim module takes `inputs` and `pkgs`. Three outputs are used:

| output | what it is |
|---|---|
| `weave.packages.${pkgs.system}.weave` | the compiler — `weave lsp`, `weave fmt`, `weave trace` |
| `weave.packages.${pkgs.system}.tree-sitter-weave` | the grammar |
| `weave.packages.${pkgs.system}.weave-nvim` | the plugin, and the tree-sitter queries |

## All four pieces at once

```nix
{ pkgs, inputs, ... }:

let
  weave = inputs.weave.packages.${pkgs.system};
in
{
  # The compiler has to be on PATH: the language server, the formatter and the
  # tracer are all subcommands of it.
  extraPackages = [ weave.weave ];

  # ---------------------------------------------------------------- grammar
  plugins.treesitter = {
    enable = true;
    grammarPackages =
      pkgs.vimPlugins.nvim-treesitter.passthru.allGrammars
      ++ [ weave.tree-sitter-weave ];
  };

  # ------------------------------------------------- the plugin and its queries
  #
  # weave.nvim carries queries/weave/highlights.scm, so installing it is what
  # makes the grammar above actually colour anything.
  extraPlugins = [ weave.weave-nvim ];

  extraConfigLua = ''
    require("weave").setup({
      -- The compiler is on PATH from extraPackages above.
      cmd = "weave",
      -- The language server is started below rather than here, so that it
      -- goes through the same machinery as every other server you have.
      lsp = false,
    })

    -- ------------------------------------------------------------------ LSP
    --
    -- `weave lsp` speaks the protocol on stdio and needs no lspconfig entry:
    -- everything it reports comes from the compiler's own front end, so the
    -- editor and `weave check` cannot disagree.
    -- `markdown` is in the list on purpose: the server reads ```weave fences
    -- inside a Markdown file as programs, so hover, completion and diagnostics
    -- work in a fence the same way they do in a .weave file.
    vim.lsp.config.weave = {
      cmd = { "weave", "lsp" },
      filetypes = { "weave", "markdown" },
      root_markers = { ".git" },
    }
    vim.lsp.enable("weave")
  '';
}
```

That is the whole thing. Opening a `.weave` file now gives you highlighting,
diagnostics as you type, hover, completion, signature help, and each
definition's value at the end of its line.

## Formatting

`weave lsp` implements `textDocument/formatting`, so **the language server is
the formatter** — no second tool, and no chance of the two disagreeing. Format
on save:

```nix
autoCmd = [{
  event = [ "BufWritePre" ];
  pattern = [ "*.weave" ];
  callback.__raw = ''
    function() vim.lsp.buf.format({ async = false }) end
  '';
}];
```

If you route every language through conform.nvim instead:

```nix
plugins.conform-nvim = {
  enable = true;
  settings = {
    formatters_by_ft.weave = [ "weave_fmt" ];
    formatters.weave_fmt = {
      command = "weave";
      args = [ "fmt" "-" ];   # `-` formats stdin to stdout
    };
  };
};
```

`weave fmt` has no options. It throws the layout away and prints the syntax
tree, so there is nothing to configure and no way for two people's files to
come out differently.

## ```weave blocks in Markdown

The grammar registers itself under the language name `weave`, so once
`plugins.treesitter` has it and the queries are on the runtimepath, a fenced
block in Markdown is highlighted with no further configuration:

````markdown
```weave
Direction is North | South | East | West

turn North is East
turn East is South
turn South is West
turn West is North

turn North
```
````

The language server reads them too. It finds the fences itself and checks each
as its own program, so hover, completion, signature help and diagnostics work
inside a block — with diagnostics reported at the line they are on in the
document, not in the block. `:WeaveTrace` in a Markdown buffer annotates every
block's definitions the same way it does a `.weave` file.

Formatting is the exception: `weave fmt` is not run over a Markdown file, since
rewriting only the fenced parts of a document would fight with whatever else
formats it.

## Comments and indentation

The plugin ships an `ftplugin/weave.lua` that sets `commentstring` to `# %s`,
so `gcc` and every other comment mapping work, and turns tabs into two spaces
— the layout rule counts columns, so a tab would mean one thing to the editor
and another to the compiler.

Nothing to configure: `extraPlugins = [ weave.weave-nvim ]` above is enough.

## Tracing, more precisely

`require("weave").setup()` turns on the ghost text for every Weave buffer and
re-runs it on write. It runs `weave trace` over the buffer with **the largest
file in the program's own directory** on stdin, which for Advent of Code is
always the real input rather than the sample you pasted in.

```lua
require("weave").setup({
  auto = false,             -- do not start until :WeaveTrace
  on_save = true,
  input_patterns = { "*.txt", "*.in", "*.input", "input*" },
  timeout_ms = 5000,
  prefix = "  = ",
  highlight = "WeaveTrace", -- links to Comment unless you set it
  max_width = 120,
})
```

`:WeaveInput` reports which file it picked, which is the first thing to check
when the numbers look wrong.

## Without nix

The same four pieces, by hand:

```lua
-- lazy.nvim
{
  "malleum/weave",
  config = function() require("weave").setup({ lsp = true }) end,
},
```

with `weave` on PATH, and the grammar registered with nvim-treesitter:

```lua
require("nvim-treesitter.parsers").get_parser_configs().weave = {
  install_info = {
    url = "https://github.com/malleum/weave",
    location = "tree-sitter-weave",
    files = { "src/parser.c", "src/scanner.c" },
  },
  filetype = "weave",
}
```
