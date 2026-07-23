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
-- Name: org; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA org;


--
-- Name: rbac; Type: SCHEMA; Schema: -; Owner: -
--

CREATE SCHEMA rbac;


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


--
-- Name: protect_control_project(); Type: FUNCTION; Schema: org; Owner: -
--

CREATE FUNCTION org.protect_control_project() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    IF TG_OP = 'DELETE' THEN
        IF OLD.is_control THEN
            RAISE EXCEPTION 'control project "%" cannot be deleted', OLD.name;
        END IF;
        RETURN OLD;
    END IF;

    IF NEW.is_control IS DISTINCT FROM OLD.is_control THEN
        RAISE EXCEPTION 'is_control cannot be changed after creation (project "%")', OLD.name;
    END IF;
    RETURN NEW;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: org_membership; Type: TABLE; Schema: org; Owner: -
--

CREATE TABLE org.org_membership (
    user_id integer NOT NULL,
    org_id integer NOT NULL
);


--
-- Name: organizations; Type: TABLE; Schema: org; Owner: -
--

CREATE TABLE org.organizations (
    id integer NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone
);


--
-- Name: organizations_id_seq; Type: SEQUENCE; Schema: org; Owner: -
--

CREATE SEQUENCE org.organizations_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: organizations_id_seq; Type: SEQUENCE OWNED BY; Schema: org; Owner: -
--

ALTER SEQUENCE org.organizations_id_seq OWNED BY org.organizations.id;


--
-- Name: projects; Type: TABLE; Schema: org; Owner: -
--

CREATE TABLE org.projects (
    id integer NOT NULL,
    org_id integer NOT NULL,
    name text NOT NULL,
    is_control boolean NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone
);


--
-- Name: projects_id_seq; Type: SEQUENCE; Schema: org; Owner: -
--

CREATE SEQUENCE org.projects_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: projects_id_seq; Type: SEQUENCE OWNED BY; Schema: org; Owner: -
--

ALTER SEQUENCE org.projects_id_seq OWNED BY org.projects.id;


--
-- Name: schema_migrations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.schema_migrations (
    version character varying NOT NULL
);


--
-- Name: custom_role_permissions; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.custom_role_permissions (
    role_id integer NOT NULL,
    permission_id integer NOT NULL
);


--
-- Name: custom_roles; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.custom_roles (
    id integer NOT NULL,
    external_id uuid NOT NULL,
    name text NOT NULL,
    org_id integer NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone,
    etag uuid NOT NULL
);


--
-- Name: custom_roles_id_seq; Type: SEQUENCE; Schema: rbac; Owner: -
--

CREATE SEQUENCE rbac.custom_roles_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: custom_roles_id_seq; Type: SEQUENCE OWNED BY; Schema: rbac; Owner: -
--

ALTER SEQUENCE rbac.custom_roles_id_seq OWNED BY rbac.custom_roles.id;


--
-- Name: org_role_assignments; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.org_role_assignments (
    user_id integer NOT NULL,
    role_id integer NOT NULL,
    org_id integer NOT NULL
);


--
-- Name: permissions; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.permissions (
    id integer NOT NULL,
    name text NOT NULL
);


--
-- Name: permissions_id_seq; Type: SEQUENCE; Schema: rbac; Owner: -
--

CREATE SEQUENCE rbac.permissions_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: permissions_id_seq; Type: SEQUENCE OWNED BY; Schema: rbac; Owner: -
--

ALTER SEQUENCE rbac.permissions_id_seq OWNED BY rbac.permissions.id;


--
-- Name: project_role_assignments; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.project_role_assignments (
    user_id integer NOT NULL,
    role_id integer NOT NULL,
    project_id integer NOT NULL
);


--
-- Name: system_role_assignments; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.system_role_assignments (
    user_id integer NOT NULL,
    role_id integer NOT NULL
);


--
-- Name: system_role_permissions; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.system_role_permissions (
    role_id integer NOT NULL,
    permission_id integer NOT NULL
);


--
-- Name: system_roles; Type: TABLE; Schema: rbac; Owner: -
--

CREATE TABLE rbac.system_roles (
    id integer NOT NULL,
    external_id uuid NOT NULL,
    name text NOT NULL,
    created_at timestamp with time zone NOT NULL,
    updated_at timestamp with time zone
);


--
-- Name: system_roles_id_seq; Type: SEQUENCE; Schema: rbac; Owner: -
--

CREATE SEQUENCE rbac.system_roles_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: system_roles_id_seq; Type: SEQUENCE OWNED BY; Schema: rbac; Owner: -
--

ALTER SEQUENCE rbac.system_roles_id_seq OWNED BY rbac.system_roles.id;


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
    email_verified_at timestamp with time zone,
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
-- Name: organizations id; Type: DEFAULT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.organizations ALTER COLUMN id SET DEFAULT nextval('org.organizations_id_seq'::regclass);


--
-- Name: projects id; Type: DEFAULT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.projects ALTER COLUMN id SET DEFAULT nextval('org.projects_id_seq'::regclass);


--
-- Name: custom_roles id; Type: DEFAULT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_roles ALTER COLUMN id SET DEFAULT nextval('rbac.custom_roles_id_seq'::regclass);


--
-- Name: permissions id; Type: DEFAULT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.permissions ALTER COLUMN id SET DEFAULT nextval('rbac.permissions_id_seq'::regclass);


--
-- Name: system_roles id; Type: DEFAULT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_roles ALTER COLUMN id SET DEFAULT nextval('rbac.system_roles_id_seq'::regclass);


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
-- Name: org_membership org_membership_pkey; Type: CONSTRAINT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.org_membership
    ADD CONSTRAINT org_membership_pkey PRIMARY KEY (user_id, org_id);


--
-- Name: organizations organizations_name_key; Type: CONSTRAINT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.organizations
    ADD CONSTRAINT organizations_name_key UNIQUE (name);


--
-- Name: organizations organizations_pkey; Type: CONSTRAINT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.organizations
    ADD CONSTRAINT organizations_pkey PRIMARY KEY (id);


--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);


--
-- Name: schema_migrations schema_migrations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.schema_migrations
    ADD CONSTRAINT schema_migrations_pkey PRIMARY KEY (version);


--
-- Name: custom_role_permissions custom_role_permissions_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_role_permissions
    ADD CONSTRAINT custom_role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: custom_roles custom_roles_etag_key; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_roles
    ADD CONSTRAINT custom_roles_etag_key UNIQUE (etag);


--
-- Name: custom_roles custom_roles_external_id_key; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_roles
    ADD CONSTRAINT custom_roles_external_id_key UNIQUE (external_id);


--
-- Name: custom_roles custom_roles_name_key; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_roles
    ADD CONSTRAINT custom_roles_name_key UNIQUE (name);


--
-- Name: custom_roles custom_roles_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_roles
    ADD CONSTRAINT custom_roles_pkey PRIMARY KEY (id);


--
-- Name: org_role_assignments org_role_assignments_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.org_role_assignments
    ADD CONSTRAINT org_role_assignments_pkey PRIMARY KEY (user_id, role_id, org_id);


--
-- Name: permissions permissions_name_key; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.permissions
    ADD CONSTRAINT permissions_name_key UNIQUE (name);


--
-- Name: permissions permissions_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.permissions
    ADD CONSTRAINT permissions_pkey PRIMARY KEY (id);


--
-- Name: project_role_assignments project_role_assignments_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.project_role_assignments
    ADD CONSTRAINT project_role_assignments_pkey PRIMARY KEY (user_id, role_id, project_id);


--
-- Name: system_role_assignments system_role_assignments_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_role_assignments
    ADD CONSTRAINT system_role_assignments_pkey PRIMARY KEY (user_id, role_id);


--
-- Name: system_role_permissions system_role_permissions_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_role_permissions
    ADD CONSTRAINT system_role_permissions_pkey PRIMARY KEY (role_id, permission_id);


--
-- Name: system_roles system_roles_external_id_key; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_roles
    ADD CONSTRAINT system_roles_external_id_key UNIQUE (external_id);


--
-- Name: system_roles system_roles_name_key; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_roles
    ADD CONSTRAINT system_roles_name_key UNIQUE (name);


--
-- Name: system_roles system_roles_pkey; Type: CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_roles
    ADD CONSTRAINT system_roles_pkey PRIMARY KEY (id);


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
-- Name: projects_org_id_control_key; Type: INDEX; Schema: org; Owner: -
--

CREATE UNIQUE INDEX projects_org_id_control_key ON org.projects USING btree (org_id) WHERE is_control;


--
-- Name: projects_org_id_lower_name_key; Type: INDEX; Schema: org; Owner: -
--

CREATE UNIQUE INDEX projects_org_id_lower_name_key ON org.projects USING btree (org_id, lower(name));


--
-- Name: org_role_assignments_user_id_org_id_idx; Type: INDEX; Schema: rbac; Owner: -
--

CREATE INDEX org_role_assignments_user_id_org_id_idx ON rbac.org_role_assignments USING btree (user_id, org_id);


--
-- Name: project_role_assignments_user_id_project_id_idx; Type: INDEX; Schema: rbac; Owner: -
--

CREATE INDEX project_role_assignments_user_id_project_id_idx ON rbac.project_role_assignments USING btree (user_id, project_id);


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
-- Name: projects protect_control_project; Type: TRIGGER; Schema: org; Owner: -
--

CREATE TRIGGER protect_control_project BEFORE DELETE OR UPDATE ON org.projects FOR EACH ROW EXECUTE FUNCTION org.protect_control_project();


--
-- Name: org_membership org_membership_org_id_fkey; Type: FK CONSTRAINT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.org_membership
    ADD CONSTRAINT org_membership_org_id_fkey FOREIGN KEY (org_id) REFERENCES org.organizations(id);


--
-- Name: org_membership org_membership_user_id_fkey; Type: FK CONSTRAINT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.org_membership
    ADD CONSTRAINT org_membership_user_id_fkey FOREIGN KEY (user_id) REFERENCES useraccess.users(id);


--
-- Name: projects projects_org_id_fkey; Type: FK CONSTRAINT; Schema: org; Owner: -
--

ALTER TABLE ONLY org.projects
    ADD CONSTRAINT projects_org_id_fkey FOREIGN KEY (org_id) REFERENCES org.organizations(id);


--
-- Name: custom_role_permissions custom_role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_role_permissions
    ADD CONSTRAINT custom_role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES rbac.permissions(id);


--
-- Name: custom_role_permissions custom_role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_role_permissions
    ADD CONSTRAINT custom_role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES rbac.custom_roles(id);


--
-- Name: custom_roles custom_roles_org_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.custom_roles
    ADD CONSTRAINT custom_roles_org_id_fkey FOREIGN KEY (org_id) REFERENCES org.organizations(id);


--
-- Name: org_role_assignments org_role_assignments_org_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.org_role_assignments
    ADD CONSTRAINT org_role_assignments_org_id_fkey FOREIGN KEY (org_id) REFERENCES org.organizations(id);


--
-- Name: org_role_assignments org_role_assignments_role_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.org_role_assignments
    ADD CONSTRAINT org_role_assignments_role_id_fkey FOREIGN KEY (role_id) REFERENCES rbac.custom_roles(id);


--
-- Name: org_role_assignments org_role_assignments_user_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.org_role_assignments
    ADD CONSTRAINT org_role_assignments_user_id_fkey FOREIGN KEY (user_id) REFERENCES useraccess.users(id);


--
-- Name: project_role_assignments project_role_assignments_project_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.project_role_assignments
    ADD CONSTRAINT project_role_assignments_project_id_fkey FOREIGN KEY (project_id) REFERENCES org.projects(id);


--
-- Name: project_role_assignments project_role_assignments_role_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.project_role_assignments
    ADD CONSTRAINT project_role_assignments_role_id_fkey FOREIGN KEY (role_id) REFERENCES rbac.custom_roles(id);


--
-- Name: project_role_assignments project_role_assignments_user_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.project_role_assignments
    ADD CONSTRAINT project_role_assignments_user_id_fkey FOREIGN KEY (user_id) REFERENCES useraccess.users(id);


--
-- Name: system_role_assignments system_role_assignments_role_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_role_assignments
    ADD CONSTRAINT system_role_assignments_role_id_fkey FOREIGN KEY (role_id) REFERENCES rbac.system_roles(id);


--
-- Name: system_role_assignments system_role_assignments_user_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_role_assignments
    ADD CONSTRAINT system_role_assignments_user_id_fkey FOREIGN KEY (user_id) REFERENCES useraccess.users(id);


--
-- Name: system_role_permissions system_role_permissions_permission_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_role_permissions
    ADD CONSTRAINT system_role_permissions_permission_id_fkey FOREIGN KEY (permission_id) REFERENCES rbac.permissions(id);


--
-- Name: system_role_permissions system_role_permissions_role_id_fkey; Type: FK CONSTRAINT; Schema: rbac; Owner: -
--

ALTER TABLE ONLY rbac.system_role_permissions
    ADD CONSTRAINT system_role_permissions_role_id_fkey FOREIGN KEY (role_id) REFERENCES rbac.system_roles(id);


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
    ('20260613000002'),
    ('20260713145900'),
    ('20260713145901'),
    ('20260713150000'),
    ('20260713150001'),
    ('20260713150002'),
    ('20260714120000'),
    ('20260714130002'),
    ('20260714130004'),
    ('20260716134326'),
    ('20260716180931');
