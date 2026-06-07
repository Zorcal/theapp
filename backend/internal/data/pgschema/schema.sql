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


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL
);


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
-- Name: users id; Type: DEFAULT; Schema: useraccess; Owner: -
--

ALTER TABLE ONLY useraccess.users ALTER COLUMN id SET DEFAULT nextval('useraccess.users_id_seq'::regclass);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


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
-- PostgreSQL database dump complete
--

\unrestrict dbmate


--
-- Dbmate schema migrations
--

INSERT INTO public.schema_migrations (version) VALUES
    ('20260605070455'),
    ('20260605070602');
