--
-- PostgreSQL database dump
--

-- Dumped from database version 12.22 (Debian 12.22-1.pgdg120+1)
-- Dumped by pg_dump version 12.22 (Debian 12.22-1.pgdg120+1)

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner:
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: users; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.users (
    id bigint NOT NULL,
    name character varying,
    itsc character varying NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    is_admin boolean DEFAULT false NOT NULL
);


ALTER TABLE public.users OWNER TO zinc;

--
-- Name: check_user_has_teaching_role(public.users); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.check_user_has_teaching_role(user_row public.users) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    hasTeachingRole BOOLEAN;
    roleCount INTEGER;
BEGIN
    roleCount := COUNT(*) FROM public.course_user WHERE user_id=user_row.id AND permission > 1;
    CASE
        WHEN roleCount > 0 THEN
            hasTeachingRole := true;
        ELSE
            hasTeachingRole := false;
    END CASE;
    RETURN hasTeachingRole;
END;
$$;


ALTER FUNCTION public.check_user_has_teaching_role(user_row public.users) OWNER TO zinc;

--
-- Name: semesters; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.semesters (
    id bigint NOT NULL,
    year integer NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    name character varying
);


ALTER TABLE public.semesters OWNER TO zinc;

--
-- Name: TABLE semesters; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TABLE public.semesters IS 'Semester';


--
-- Name: get_academic_term(public.semesters); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.get_academic_term(semester_row public.semesters) RETURNS character varying
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    term varchar;
BEGIN
    CASE
        WHEN right(semester_row.id::varchar(32), 2)='10' THEN
            term := 'FALL';
        WHEN right(semester_row.id::varchar(32), 2)='20' THEN
            term := 'WINTER';
        WHEN right(semester_row.id::varchar(32), 2)='30' THEN
            term := 'SPRING';
        WHEN right(semester_row.id::varchar(32), 2)='40' THEN
            term := 'SUMMER';
        ELSE
            term := 'UNKNOWN';
    END CASE;
    RETURN term;
END;
$$;


ALTER FUNCTION public.get_academic_term(semester_row public.semesters) OWNER TO zinc;

--
-- Name: course_user; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.course_user (
    id bigint NOT NULL,
    course_id bigint NOT NULL,
    user_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    permission integer DEFAULT 0 NOT NULL
);


ALTER TABLE public.course_user OWNER TO zinc;

--
-- Name: get_user_course_role(public.course_user); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.get_user_course_role(user_row public.course_user) RETURNS character varying
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    role VARCHAR;
BEGIN
    CASE
        WHEN user_row.permission > 1 THEN
            role := 'Teaching Staff';
        ELSE
            role := 'Student';
    END CASE;
    RETURN role;
END;
$$;


ALTER FUNCTION public.get_user_course_role(user_row public.course_user) OWNER TO zinc;

--
-- Name: get_user_initials(public.users); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.get_user_initials(user_row public.users) RETURNS character varying
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    initials VARCHAR;
    name_partitions TEXT ARRAY;
BEGIN
    name_partitions := string_to_array(user_row.name, ' ');
    CASE
        WHEN name_partitions[1] LIKE '%,%' THEN
            CASE
                WHEN array_length(name_partitions, 1)-1 > 2 THEN
                    initials := substring(name_partitions[1],1,1);
                WHEN array_length(name_partitions, 1)-1 = 2 THEN
                    initials := concat(substring(name_partitions[2],1,1), substring(name_partitions[3],1,1));
                ELSE
                    initials := concat(substring(name_partitions[1],1,1), substring(name_partitions[array_length(name_partitions, 1)],1,1));
            END CASE;
        WHEN array_length(name_partitions, 1) > 3 OR array_length(name_partitions, 1) = 1 THEN
            initials := substring(name_partitions[1],1,1);
        ELSE
            initials := concat(substring(name_partitions[1],1,1), substring(name_partitions[array_length(name_partitions, 1)],1,1));
    END CASE;
    RETURN initials;
END;
$$;


ALTER FUNCTION public.get_user_initials(user_row public.users) OWNER TO zinc;

--
-- Name: get_user_role(public.course_user); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.get_user_role(user_row public.course_user) RETURNS character varying
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    role VARCHAR;
BEGIN
    CASE
        WHEN user_row.permission = 1 THEN
            role := 'Student';
        ELSE
            role := 'Teaching Staff';
    END CASE;
    RETURN role;
END;
$$;


ALTER FUNCTION public.get_user_role(user_row public.course_user) OWNER TO zinc;

--
-- Name: submissions; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.submissions (
    id bigint NOT NULL,
    stored_name character varying NOT NULL,
    upload_name character varying NOT NULL,
    extracted_path character varying,
    size integer NOT NULL,
    checksum character varying NOT NULL,
    fail_reason character varying,
    remarks json,
    assignment_config_id bigint NOT NULL,
    user_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.submissions OWNER TO zinc;

--
-- Name: is_submission_late(public.submissions); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.is_submission_late(submission_row public.submissions) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    is_late BOOLEAN;
    assignment_due_at timestamp;
BEGIN
    SELECT due_at INTO assignment_due_at FROM assignment_configs WHERE id = submission_row.assignment_config_id;
    CASE
        WHEN submission_row.created_at > assignment_due_at THEN
            is_late := true;
        ELSE
            is_late := false;
    END CASE;
    RETURN is_late;

END;
$$;


ALTER FUNCTION public.is_submission_late(submission_row public.submissions) OWNER TO zinc;

--
-- Name: assignment_configs; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.assignment_configs (
    id bigint NOT NULL,
    show_at timestamp without time zone,
    start_collection_at timestamp without time zone,
    due_at timestamp without time zone NOT NULL,
    stop_collection_at timestamp without time zone NOT NULL,
    release_grade_at timestamp without time zone,
    attempt_limits integer,
    assignment_id bigint NOT NULL,
    is_tested boolean NOT NULL,
    config_yaml text NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    show_immediate_scores boolean DEFAULT false NOT NULL,
    grade_immediately boolean DEFAULT false NOT NULL,
    CONSTRAINT submission_stopcollection_after_startcollection CHECK ((start_collection_at < stop_collection_at))
);


ALTER TABLE public.assignment_configs OWNER TO zinc;

--
-- Name: open_for_submission(public.assignment_configs); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.open_for_submission(assignment_config_row public.assignment_configs) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
BEGIN
    CASE
        WHEN assignment_config_row.start_collection_at is null THEN
            RETURN false;
        ELSE
            RETURN assignment_config_row.start_collection_at < now();
    END CASE;
END;
$$;


ALTER FUNCTION public.open_for_submission(assignment_config_row public.assignment_configs) OWNER TO zinc;

--
-- Name: set_current_timestamp_updated_at(); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.set_current_timestamp_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
  _new record;
BEGIN
  _new := NEW;
  _new."updated_at" = NOW();
  RETURN _new;
END;
$$;


ALTER FUNCTION public.set_current_timestamp_updated_at() OWNER TO zinc;

--
-- Name: submission_window_passed(public.assignment_configs); Type: FUNCTION; Schema: public; Owner: zinc
--

CREATE FUNCTION public.submission_window_passed(assignment_config_row public.assignment_configs) RETURNS boolean
    LANGUAGE plpgsql STABLE
    AS $$
DECLARE
    submissionWindowPassed BOOLEAN;
BEGIN
    RETURN assignment_config_row.stop_collection_at < now();
END;
$$;


ALTER FUNCTION public.submission_window_passed(assignment_config_row public.assignment_configs) OWNER TO zinc;


--
-- Name: assignment_config_user; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.assignment_config_user (
    assignment_config_id bigint NOT NULL,
    user_id bigint NOT NULL,
    id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.assignment_config_user OWNER TO zinc;

--
-- Name: TABLE assignment_config_user; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TABLE public.assignment_config_user IS 'AssignmentConfig-User Mapping';


--
-- Name: assignment_config_user_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.assignment_config_user_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.assignment_config_user_id_seq OWNER TO zinc;

--
-- Name: assignment_config_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.assignment_config_user_id_seq OWNED BY public.assignment_config_user.id;


--
-- Name: assignment_configs_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.assignment_configs_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.assignment_configs_id_seq OWNER TO zinc;

--
-- Name: assignment_configs_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.assignment_configs_id_seq OWNED BY public.assignment_configs.id;


--
-- Name: assignment_types; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.assignment_types (
    id integer NOT NULL,
    name character varying NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


ALTER TABLE public.assignment_types OWNER TO zinc;

--
-- Name: assignment_types_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.assignment_types_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.assignment_types_id_seq OWNER TO zinc;

--
-- Name: assignment_types_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.assignment_types_id_seq OWNED BY public.assignment_types.id;


--
-- Name: assignments; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.assignments (
    id bigint NOT NULL,
    name character varying NOT NULL,
    description text NOT NULL,
    description_html text NOT NULL,
    course_id bigint NOT NULL,
    show_at timestamp without time zone,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    type bigint DEFAULT 2 NOT NULL
);


ALTER TABLE public.assignments OWNER TO zinc;

--
-- Name: assignments_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.assignments_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.assignments_id_seq OWNER TO zinc;

--
-- Name: assignments_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.assignments_id_seq OWNED BY public.assignments.id;


--
-- Name: course_user_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.course_user_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.course_user_id_seq OWNER TO zinc;

--
-- Name: course_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.course_user_id_seq OWNED BY public.course_user.id;


--
-- Name: courses; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.courses (
    id bigint NOT NULL,
    code character varying NOT NULL,
    name character varying NOT NULL,
    is_shown boolean DEFAULT false NOT NULL,
    semester_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone,
    website character varying
);


ALTER TABLE public.courses OWNER TO zinc;

--
-- Name: courses_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.courses_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.courses_id_seq OWNER TO zinc;

--
-- Name: courses_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.courses_id_seq OWNED BY public.courses.id;


--
-- Name: reports; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.reports (
    id bigint NOT NULL,
    is_final boolean NOT NULL,
    is_test boolean NOT NULL,
    pipeline_results jsonb NOT NULL,
    show_at timestamp without time zone,
    remarks json NOT NULL,
    submission_id bigint NOT NULL,
    initiated_by bigint,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    grade jsonb,
    "sanitizedReports" jsonb
);


ALTER TABLE public.reports OWNER TO zinc;

--
-- Name: reports_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.reports_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.reports_id_seq OWNER TO zinc;

--
-- Name: reports_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.reports_id_seq OWNED BY public.reports.id;


--
-- Name: section_user; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.section_user (
    create_at timestamp without time zone DEFAULT now(),
    updated_at timestamp without time zone DEFAULT now(),
    id bigint NOT NULL,
    user_id bigint NOT NULL,
    section_id bigint NOT NULL
);


ALTER TABLE public.section_user OWNER TO zinc;

--
-- Name: section_user_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.section_user_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.section_user_id_seq OWNER TO zinc;

--
-- Name: section_user_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.section_user_id_seq OWNED BY public.section_user.id;


--
-- Name: sections; Type: TABLE; Schema: public; Owner: zinc
--

CREATE TABLE public.sections (
    id bigint NOT NULL,
    name character varying NOT NULL,
    course_id bigint NOT NULL,
    created_at timestamp without time zone DEFAULT now() NOT NULL,
    updated_at timestamp without time zone DEFAULT now() NOT NULL,
    deleted_at timestamp without time zone
);


ALTER TABLE public.sections OWNER TO zinc;

--
-- Name: sections_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.sections_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.sections_id_seq OWNER TO zinc;

--
-- Name: sections_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.sections_id_seq OWNED BY public.sections.id;


--
-- Name: submissions_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.submissions_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.submissions_id_seq OWNER TO zinc;

--
-- Name: submissions_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.submissions_id_seq OWNED BY public.submissions.id;


--
-- Name: users_id_seq; Type: SEQUENCE; Schema: public; Owner: zinc
--

CREATE SEQUENCE public.users_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


ALTER TABLE public.users_id_seq OWNER TO zinc;

--
-- Name: users_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: zinc
--

ALTER SEQUENCE public.users_id_seq OWNED BY public.users.id;


--
-- Name: assignment_config_user id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_config_user ALTER COLUMN id SET DEFAULT nextval('public.assignment_config_user_id_seq'::regclass);


--
-- Name: assignment_configs id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_configs ALTER COLUMN id SET DEFAULT nextval('public.assignment_configs_id_seq'::regclass);


--
-- Name: assignment_types id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_types ALTER COLUMN id SET DEFAULT nextval('public.assignment_types_id_seq'::regclass);


--
-- Name: assignments id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignments ALTER COLUMN id SET DEFAULT nextval('public.assignments_id_seq'::regclass);


--
-- Name: course_user id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.course_user ALTER COLUMN id SET DEFAULT nextval('public.course_user_id_seq'::regclass);


--
-- Name: courses id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.courses ALTER COLUMN id SET DEFAULT nextval('public.courses_id_seq'::regclass);


--
-- Name: reports id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.reports ALTER COLUMN id SET DEFAULT nextval('public.reports_id_seq'::regclass);


--
-- Name: section_user id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.section_user ALTER COLUMN id SET DEFAULT nextval('public.section_user_id_seq'::regclass);


--
-- Name: sections id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.sections ALTER COLUMN id SET DEFAULT nextval('public.sections_id_seq'::regclass);


--
-- Name: submissions id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.submissions ALTER COLUMN id SET DEFAULT nextval('public.submissions_id_seq'::regclass);


--
-- Name: users id; Type: DEFAULT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.users ALTER COLUMN id SET DEFAULT nextval('public.users_id_seq'::regclass);


--
-- Name: assignment_config_user assignment_config_user_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_config_user
    ADD CONSTRAINT assignment_config_user_pkey PRIMARY KEY (assignment_config_id, user_id);


--
-- Name: assignment_configs assignment_configs_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_configs
    ADD CONSTRAINT assignment_configs_pkey PRIMARY KEY (id);


--
-- Name: assignment_types assignment_types_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_types
    ADD CONSTRAINT assignment_types_pkey PRIMARY KEY (id);


--
-- Name: assignments assignments_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignments
    ADD CONSTRAINT assignments_pkey PRIMARY KEY (id);


--
-- Name: course_user course_user_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.course_user
    ADD CONSTRAINT course_user_pkey PRIMARY KEY (course_id, user_id);


--
-- Name: courses courses_code_semester_id_key; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.courses
    ADD CONSTRAINT courses_code_semester_id_key UNIQUE (code, semester_id);


--
-- Name: courses courses_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.courses
    ADD CONSTRAINT courses_pkey PRIMARY KEY (id);


--
-- Name: reports reports_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.reports
    ADD CONSTRAINT reports_pkey PRIMARY KEY (id);


--
-- Name: section_user section_user_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.section_user
    ADD CONSTRAINT section_user_pkey PRIMARY KEY (id);


--
-- Name: section_user section_user_user_id_section_id_key; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.section_user
    ADD CONSTRAINT section_user_user_id_section_id_key UNIQUE (user_id, section_id);


--
-- Name: sections sections_course_id_name_key; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.sections
    ADD CONSTRAINT sections_course_id_name_key UNIQUE (course_id, name);


--
-- Name: sections sections_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.sections
    ADD CONSTRAINT sections_pkey PRIMARY KEY (id);


--
-- Name: semesters semesters_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.semesters
    ADD CONSTRAINT semesters_pkey PRIMARY KEY (id);


--
-- Name: submissions submissions_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.submissions
    ADD CONSTRAINT submissions_pkey PRIMARY KEY (id);


--
-- Name: users users_itsc_key; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_itsc_key UNIQUE (itsc);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: assignment_id; Type: INDEX; Schema: public; Owner: zinc
--

CREATE INDEX assignment_id ON public.assignment_configs USING btree (assignment_id);


--
-- Name: submission_id; Type: INDEX; Schema: public; Owner: zinc
--

CREATE INDEX submission_id ON public.reports USING btree (submission_id);


--
-- Name: submissions_extracted_path; Type: INDEX; Schema: public; Owner: zinc
--

CREATE INDEX submissions_extracted_path ON public.submissions USING btree (extracted_path);


--
-- Name: submissions_uid_assignmentconfigid; Type: INDEX; Schema: public; Owner: zinc
--

CREATE INDEX submissions_uid_assignmentconfigid ON public.submissions USING btree (assignment_config_id, user_id);


--
-- Name: assignment_config_user set_public_assignment_config_user_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_assignment_config_user_updated_at BEFORE UPDATE ON public.assignment_config_user FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_assignment_config_user_updated_at ON assignment_config_user; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_assignment_config_user_updated_at ON public.assignment_config_user IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: assignment_configs set_public_assignment_configs_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_assignment_configs_updated_at BEFORE UPDATE ON public.assignment_configs FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_assignment_configs_updated_at ON assignment_configs; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_assignment_configs_updated_at ON public.assignment_configs IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: assignment_types set_public_assignment_types_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_assignment_types_updated_at BEFORE UPDATE ON public.assignment_types FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_assignment_types_updated_at ON assignment_types; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_assignment_types_updated_at ON public.assignment_types IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: assignments set_public_assignments_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_assignments_updated_at BEFORE UPDATE ON public.assignments FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_assignments_updated_at ON assignments; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_assignments_updated_at ON public.assignments IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: courses set_public_courses_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_courses_updated_at BEFORE UPDATE ON public.courses FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_courses_updated_at ON courses; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_courses_updated_at ON public.courses IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: reports set_public_reports_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_reports_updated_at BEFORE UPDATE ON public.reports FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_reports_updated_at ON reports; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_reports_updated_at ON public.reports IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: section_user set_public_section_user_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_section_user_updated_at BEFORE UPDATE ON public.section_user FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_section_user_updated_at ON section_user; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_section_user_updated_at ON public.section_user IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: sections set_public_sections_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_sections_updated_at BEFORE UPDATE ON public.sections FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_sections_updated_at ON sections; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_sections_updated_at ON public.sections IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: semesters set_public_semesters_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_semesters_updated_at BEFORE UPDATE ON public.semesters FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_semesters_updated_at ON semesters; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_semesters_updated_at ON public.semesters IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: submissions set_public_submissions_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_submissions_updated_at BEFORE UPDATE ON public.submissions FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_submissions_updated_at ON submissions; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_submissions_updated_at ON public.submissions IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: users set_public_users_updated_at; Type: TRIGGER; Schema: public; Owner: zinc
--

CREATE TRIGGER set_public_users_updated_at BEFORE UPDATE ON public.users FOR EACH ROW EXECUTE FUNCTION public.set_current_timestamp_updated_at();


--
-- Name: TRIGGER set_public_users_updated_at ON users; Type: COMMENT; Schema: public; Owner: zinc
--

COMMENT ON TRIGGER set_public_users_updated_at ON public.users IS 'trigger to set value of column "updated_at" to current timestamp on row update';


--
-- Name: assignment_config_user assignment_config_user_assignment_config_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_config_user
    ADD CONSTRAINT assignment_config_user_assignment_config_id_fkey FOREIGN KEY (assignment_config_id) REFERENCES public.assignment_configs(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: assignment_config_user assignment_config_user_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_config_user
    ADD CONSTRAINT assignment_config_user_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: assignment_configs assignment_configs_assignment_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignment_configs
    ADD CONSTRAINT assignment_configs_assignment_id_fkey FOREIGN KEY (assignment_id) REFERENCES public.assignments(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: assignments assignments_course_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignments
    ADD CONSTRAINT assignments_course_id_fkey FOREIGN KEY (course_id) REFERENCES public.courses(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: assignments assignments_type_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.assignments
    ADD CONSTRAINT assignments_type_fkey FOREIGN KEY (type) REFERENCES public.assignment_types(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: course_user course_user_course_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.course_user
    ADD CONSTRAINT course_user_course_id_fkey FOREIGN KEY (course_id) REFERENCES public.courses(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: course_user course_user_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.course_user
    ADD CONSTRAINT course_user_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: courses courses_semester_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.courses
    ADD CONSTRAINT courses_semester_id_fkey FOREIGN KEY (semester_id) REFERENCES public.semesters(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: reports reports_initiated_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.reports
    ADD CONSTRAINT reports_initiated_by_fkey FOREIGN KEY (initiated_by) REFERENCES public.users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: reports reports_submission_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.reports
    ADD CONSTRAINT reports_submission_id_fkey FOREIGN KEY (submission_id) REFERENCES public.submissions(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: section_user section_user_section_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.section_user
    ADD CONSTRAINT section_user_section_id_fkey FOREIGN KEY (section_id) REFERENCES public.sections(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: section_user section_user_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.section_user
    ADD CONSTRAINT section_user_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: sections sections_course_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.sections
    ADD CONSTRAINT sections_course_id_fkey FOREIGN KEY (course_id) REFERENCES public.courses(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: submissions submissions_assignment_config_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.submissions
    ADD CONSTRAINT submissions_assignment_config_id_fkey FOREIGN KEY (assignment_config_id) REFERENCES public.assignment_configs(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- Name: submissions submissions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: zinc
--

ALTER TABLE ONLY public.submissions
    ADD CONSTRAINT submissions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON UPDATE RESTRICT ON DELETE RESTRICT;


--
-- PostgreSQL database dump complete
--
