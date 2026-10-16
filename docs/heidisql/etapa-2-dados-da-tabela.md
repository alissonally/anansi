# Etapa 2 — Dados da tabela (grade editável)

**Status:** concluída.

A sub-aba **Dados** da aba de tabela/view é o equivalente da aba "Data" do HeidiSQL.

## Leitura

- O SELECT é montado **no backend** a partir de uma especificação (`database.BrowseSpec`): colunas visíveis,
  filtro WHERE digitado pelo usuário, ordenação múltipla e `LIMIT offset, n` — como o HeidiSQL, por páginas
  de 1.000 linhas ("+1.000" e "Tudo", até 100.000). O `LIMIT` deixa o servidor fazer *top-N* em vez de ordenar a
  tabela inteira.
- **Carga parcial de colunas longas** (TEXT, BLOB, JSON, VARCHAR > 1024) quando há chave: o servidor envia só
  `LEFT(col, 1000)` e um indicador `CHAR_LENGTH(col) > 1000` por coluna. O HeidiSQL também trunca, mas
  adivinha se o valor foi cortado; aqui o indicador é exato (marca `…` na célula). O valor completo (até 16 MiB)
  é lido sob demanda pela chave ao abrir o editor (`FetchRowValues`). Sem chave nada é truncado, porque todos
  os valores servem para identificar a linha.
- As colunas-chave entram sempre no SELECT, mesmo ocultas, para a linha continuar editável.
- Cada carga é cancelável (botão Cancelar, ou outra carga da mesma aba) com `KILL QUERY` + cancelamento do
  contexto; ao parar no limite de linhas a conexão é descartada em vez de drenar o resto.
- Contagem: estimativa do `information_schema` e botão **Contar** (`SELECT COUNT(*)` com o filtro).
- A navegação de dados não entra no histórico de consultas (aparece no log de SQL).

## Filtros e ordenação

- Caixa **WHERE** (o `WHERE` inicial é opcional), Enter aplica, histórico dos últimos 20 filtros por tabela.
- **Buscar texto em todas as colunas**: gera `CAST(col AS CHAR) LIKE '%texto%' OR …` (exceto binários).
- **Filtros rápidos** no menu de contexto (submenus "Filtrar por este valor" e "Adicionar ao filtro (AND)"):
  `=`, `≠`, `>`, `<`, contém, começa com, termina com, `IN`/`NOT IN` com os valores das linhas selecionadas,
  `IS NULL`, `IS NOT NULL`, "Igual a…", "Contendo…" e **Valores mais frequentes** (`GROUP BY` com contagem).
- Ordenação: clique no cabeçalho alterna crescente → decrescente → nenhuma; **Shift+clique** monta ordenação
  por várias colunas (com a posição indicada no cabeçalho).
- **Colunas**: seletor com filtro, "todas"/"nenhuma".
- Filtro, ordenação, colunas ocultas e histórico são lembrados por tabela entre sessões (opção do HeidiSQL,
  aqui sempre ativa).
- **Seguir chave estrangeira**: abre a tabela referenciada já filtrada pelo valor.

## Edição

- Edição inline: duplo clique, Enter/F2 ou simplesmente começar a digitar (como numa planilha); Tab/Enter
  confirmam e andam; Esc cancela. ENUM e booleanos (`TINYINT(1)`) usam lista; colunas com chave estrangeira
  oferecem os valores da tabela referenciada (com rótulo da primeira coluna de texto, como o HeidiSQL).
- **Editor em janela** para textos longos, JSON (validação, formatar/compactar), binários (hex, ver/editar
  como texto UTF-8, prévia de imagem PNG/JPEG/GIF/WebP/BMP), SET (caixas de seleção) e valores truncados.
  Cada valor pode ser **Valor**, **NULL**, **DEFAULT** ou **Expressão SQL** (o "SQL function" do HeidiSQL),
  com atalhos para `NOW()`, `UTC_TIMESTAMP()`, `CURDATE()`, `UUID()`, `UNHEX(REPLACE(UUID(),'-',''))` etc.
- Inserir (Insert), duplicar (Ctrl+D, sem colunas AUTO_INCREMENT/geradas), excluir (Ctrl+Delete), NULL
  (Ctrl+Shift+N), desfazer da linha (Ctrl+Z), colar TSV em várias células (Ctrl+V).
- **Edição em lote**: as alterações ficam pendentes (célula amarela, linha nova verde, excluída riscada) e
  **Ctrl+S** grava tudo numa **única transação** — melhoria sobre o HeidiSQL, que grava linha a linha. Se uma
  instrução falha, nada é gravado, a linha com problema é marcada e o erro aparece com a instrução.
- **SQL**: mostra as instruções pendentes e permite abri-las numa aba.
- Fechar a aba ou recarregar com alterações pendentes pede confirmação.
- Views são somente leitura.

### Geração do SQL de escrita (`internal/database/mysql/data.go`)

- Identificação da linha como no HeidiSQL: chave primária; senão índice UNIQUE com todas as colunas NOT NULL;
  senão **todas as colunas + `LIMIT 1`** (com aviso na tela e na hora de salvar).
- **Valores sempre como parâmetros** (`?`), nunca interpolados. O HeidiSQL escapa literais manualmente, o que
  corrompe dados quando o servidor usa `NO_BACKSLASH_ESCAPES`; o teste de integração grava
  `C:\pasta\'aspas' "duplas" ; --` com esse modo ligado e lê o mesmo valor de volta.
- Na cláusula WHERE, FLOAT/DOUBLE, data/hora, JSON e espaciais são comparados como texto
  (`CAST(col AS CHAR) = ?`), evitando erro de arredondamento e diferenças de formatação; binários por bytes.
- BIT aceita número ou `b'0101'`; binários em hex; expressões não podem conter `;`.
- Cada instrução pode afetar **no máximo uma linha**; se afetar mais, a transação inteira é revertida.
- Confirmações de produção e o bloqueio de perfis somente leitura valem para as edições; a confirmação mostra o
  SQL com os valores reais (não os `?`).

## Correção encontrada no caminho

O driver passou a devolver `FLOAT` como `float32` e a conversão para JSON mostrava `1.100000023841858` em vez
de `1.1` (afetava também a grade de consultas). Agora usa a menor representação decimal de 32 bits.

## Testes

- Unitários Go (`internal/database/mysql/data_test.go`): escolha da chave, SELECT com colunas parciais e
  chave anexada, ordenação e filtro, UPDATE/INSERT/DELETE com parâmetros, `LIMIT 1` sem chave ou com chave
  incompleta, rejeição de colunas inexistentes, expressões com `;`, hex e BIT inválidos.
- Unitários frontend (`tests/grid.test.ts`): ENUM/SET, editores por tipo, edições geradas, linhas truncadas
  nunca usadas para identificar, filtros rápidos e busca.
- Integração (`internal/integration/data_test.go`, MySQL 8.4 e MariaDB 11.4, pela API real `api.App`):
  paginação, ordenação, indicadores de truncamento exatos, valor completo, contagem, transação com
  UPDATE/INSERT/DELETE e `LAST_INSERT_ID`, `NO_BACKSLASH_ESCAPES`, rollback em falha, tabela sem chave com
  linhas duplicadas (só uma alterada), valores frequentes, lookup de FK, erro de sintaxe no filtro, perfil
  somente leitura e confirmação em produção.
- Verificação no app (Chrome headless): edição, inserção, gravação, erro de chave duplicada com rollback,
  filtros rápidos, editor JSON e prévia do SQL.

## Pendências (para etapas seguintes)

- Inserir arquivos em BLOBs e salvar BLOB em arquivo → Etapa 5 (arquivos).
- "Coluna com timestamp UNIX" exibida como data → Etapa 8 (preferências).
- Exportar/copiar como os formatos de exportação → Etapa 5.
