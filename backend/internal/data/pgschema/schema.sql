\restrict dbmate

-- Dumped from database version 17.10 (Debian 17.10-1.pgdg13+1)
-- Dumped by pg_dump version 18.3

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET transaction_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: useraccess; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA useraccess;


--
-- Name: pg_trgm; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pg_trgm WITH SCHEMA public;


--
-- Name: EXTENSION pg_trgm; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pg_trgm IS 'text similarity measurement and index searching based on trigrams';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL
);


--
-- Name: magic_link_tokens; Type: TABLE; Schema: useraccess; Owner: -
--

CREATE TABLE useraccess.magic_link_tokens (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: magic_link_tokens_id_seq; Type: SEQUENCE; Schema: useraccess; Owner: -
--

CREATE SEQUENCE useraccess.magic_link_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: magic_link_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: useraccess; Owner: -
--

ALTER SEQUENCE useraccess.magic_link_tokens_id_seq OWNED BY useraccess.magic_link_tokens.id;


--
-- Name: refresh_tokens; Type: TABLE; Schema: useraccess; Owner: -
--

CREATE TABLE useraccess.refresh_tokens (
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    token_hash text NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    revoked_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: refresh_tokens_id_seq; Type: SEQUENCE; Schema: useraccess; Owner: -
--

CREATE SEQUENCE useraccess.refresh_tokens_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: refresh_tokens_id_seq; Type: SEQUENCE OWNED BY; Schema: useraccess; Owner: -
--

ALTER SEQUENCE useraccess.refresh_tokens_id_seq OWNED BY useraccess.refresh_tokens.id;


--
-- Name: users; Type: TABLE; Schema: useraccess; Owner: -
--

CREATE TABLE useraccess.users (
    id integer NOT NULL,
    external_id uuid NOT NULL,
    email text NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone,
    etag uuid NOT NULL
);


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: useraccess; Owner: -
--

CREATE SEQUENCE useraccess.users_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: useraccess; Owner: -
--

ALTER SEQUENCE useraccess.users_id_seq OWNED BY useraccess.users.id;


--
-- Name: magic_link_tokens id; Type: DEFAULT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.magic_link_tokens ALTER COLUMN id SET DEFAULT nextval('useraccess.magic_link_tokens_id_seq'::regclass);


--
-- Name: refresh_tokens id; Type: DEFAULT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.refresh_tokens ALTER COLUMN id SET DEFAULT nextval('useraccess.refresh_tokens_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.users ALTER COLUMN id SET DEFAULT nextval('useraccess.users_id_seq'::regclass);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: magic_link_tokens magic_link_tokens_pkey; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.magic_link_tokens
    ADD CONSTRAINT magic_link_tokens_pkey PRIMARY KEY (id);


--
-- Name: magic_link_tokens magic_link_tokens_token_hash_key; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.magic_link_tokens
    ADD CONSTRAINT magic_link_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: refresh_tokens refresh_tokens_pkey; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.refresh_tokens
    ADD CONSTRAINT refresh_tokens_pkey PRIMARY KEY (id);


--
-- Name: refresh_tokens refresh_tokens_token_hash_key; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.refresh_tokens
    ADD CONSTRAINT refresh_tokens_token_hash_key UNIQUE (token_hash);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_etag_key; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.users
    ADD CONSTRAINT users_etag_key UNIQUE (etag);


--
-- Name: users users_external_id_key; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.users
    ADD CONSTRAINT users_external_id_key UNIQUE (external_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: magic_link_tokens_user_id_created_at_idx; Type: INDEX; Schema: useraccess; Owner: -
--

CREATE INDEX magic_link_tokens_user_id_created_at_idx ON useraccess.magic_link_tokens USING btree (user_id, created_at DESC);


--
-- Name: refresh_tokens_user_id_idx; Type: INDEX; Schema: useraccess; Owner: -
--

CREATE INDEX refresh_tokens_user_id_idx ON useraccess.refresh_tokens USING btree (user_id);


--
-- Name: users_created_at_idx; Type: INDEX; Schema: useraccess; Owner: -
--

CREATE INDEX users_created_at_idx ON useraccess.users USING btree (created_at);


--
-- Name: users_email_trgm_idx; Type: INDEX; Schema: useraccess; Owner: -
--

CREATE INDEX users_email_trgm_idx ON useraccess.users USING gin (email public.gin_trgm_ops);


--
-- Name: users_name_trgm_idx; Type: INDEX; Schema: useraccess; Owner: -
--

CREATE INDEX users_name_trgm_idx ON useraccess.users USING gin (name public.gin_trgm_ops);


--
-- Name: users_updated_at_idx; Type: INDEX; Schema: useraccess; Owner: -
--

CREATE INDEX users_updated_at_idx ON useraccess.users USING btree (updated_at);


--
-- Name: magic_link_tokens magic_link_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.magic_link_tokens
    ADD CONSTRAINT magic_link_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES useraccess.users(id);


--
-- Name: refresh_tokens refresh_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.refresh_tokens
    ADD CONSTRAINT refresh_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES useraccess.users(id);


--
-- PostgreSQL database dump complete
--

\unrestrict dbmate


--
-- Dbmate schema migrations
--

INSERT INTO public.schema_migrations (version) VALUES
    ('20260605070455'),
    ('20260605070601'),
    ('20260605070602'),
    ('20260613000001'),
    ('20260613000002');
