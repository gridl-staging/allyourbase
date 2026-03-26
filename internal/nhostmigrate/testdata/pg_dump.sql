CREATE TABLE public.authors (
    id bigint PRIMARY KEY,
    name text NOT NULL
);

CREATE TABLE public.posts (
    id bigint PRIMARY KEY,
    author_id bigint NOT NULL,
    title text NOT NULL
);

CREATE TABLE hdb_catalog.hdb_table (
    id bigint PRIMARY KEY
);

CREATE INDEX posts_title_idx ON public.posts USING btree (title);

INSERT INTO public.authors (id, name) VALUES (1, 'Alice');
INSERT INTO public.posts (id, author_id, title) VALUES (10, 1, 'Hello');
