// seeddev adds repeatable sample correspondence to a local development database.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	domainmessages "github.com/Kapital-B/automata/svc/internal/domain/messages"
	"log"
	"math/rand"
	"net"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/factory"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/postgres"
	"github.com/Kapital-B/automata/svc/internal/adapters/outbound/persistence/sqlite"
	"github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainaccounts "github.com/Kapital-B/automata/svc/internal/domain/accounts"
	domainprojects "github.com/Kapital-B/automata/svc/internal/domain/projects"
	"github.com/google/uuid"
)

type repository interface {
	GetUserByEmail(context.Context, string) (uuid.UUID, *string, error)
	GetHomeOrganisationID(context.Context, uuid.UUID) (uuid.UUID, error)
	GetAccount(context.Context, uuid.UUID, uuid.UUID) (*domainaccounts.AccountRow, []byte, error)
	InsertAccount(context.Context, domainaccounts.AccountRow, []byte) error
	GetProjectByCode(context.Context, uuid.UUID, string) (*domainprojects.ProjectRow, error)
	CreateProject(context.Context, domainprojects.ProjectRow, domainprojects.ProjectMemberRow) error
	GetProjectMember(context.Context, uuid.UUID, uuid.UUID) (*domainprojects.ProjectMemberRow, error)
	UpsertMessage(context.Context, domainmessages.MessageRow) error
	UpsertThreadAssignment(context.Context, domainprojects.AssignmentRow) error
}

type scenario struct {
	name, client, description string
	keywords, topics          []string
	people                    []person
}
type person struct{ name, address string }

var scenarios = []scenario{
	{"Harbour View Solar Retrofit", "Harbour View Properties", "Rooftop solar and battery installation across the Harbour View office campus.", []string{"solar", "inverter", "roof", "battery"}, []string{"Roof load survey", "Inverter specification", "Grid connection application", "Battery room ventilation", "Commissioning schedule", "Metering drawings", "Fire officer comments", "Panel delivery", "Handover pack"}, []person{{"Maya Patel", "maya.patel@harbourview.example"}, {"Oliver Reed", "oliver.reed@solaris.example"}, {"Nandi Mokoena", "nandi.mokoena@gridworks.example"}}},
	{"Riverside Clinic Expansion", "Riverside Health", "New outpatient wing with phased works around the operating clinic.", []string{"clinic", "outpatient", "MEP", "phasing"}, []string{"Outpatient layout", "Sterile corridor", "Air handling capacity", "Weekend shutdown", "Medical gas routing", "Accessibility review", "Reception phasing", "Generator changeover", "Practical completion"}, []person{{"Sarah Chen", "sarah.chen@riverside.example"}, {"Daniel Price", "daniel.price@carebuild.example"}, {"Amina Dlamini", "amina.dlamini@designlab.example"}}},
	{"Eastbank Water Upgrade", "Eastbank Municipality", "Pump station refurbishment and new monitoring equipment for the eastern supply zone.", []string{"pump", "water", "SCADA", "reservoir"}, []string{"Pump P-03 duty", "Valve chamber survey", "SCADA point list", "Reservoir outage", "Motor starter drawings", "Pressure test", "Telemetry cabinet", "Spare parts schedule", "Operator training"}, []person{{"Thabo Ndlovu", "thabo.ndlovu@eastbank.example"}, {"Priya Naidoo", "priya.naidoo@hydrotech.example"}, {"James Walker", "james.walker@controls.example"}}},
	{"North Quay Logistics Hub", "North Quay Logistics", "Warehouse fit-out, yard circulation and loading dock commissioning.", []string{"warehouse", "dock", "yard", "racking"}, []string{"Dock leveller submittal", "Racking clearance", "Yard drainage", "Traffic management", "Fire door schedule", "Cold room power", "Sprinkler test", "Forklift charging", "Operations handover"}, []person{{"Leila Jacobs", "leila.jacobs@northquay.example"}, {"Ben Carter", "ben.carter@buildline.example"}, {"Fatima Khan", "fatima.khan@logisticsdesign.example"}}},
	{"Cedar Ridge School Renewal", "Cedar Ridge Academy", "Classroom renewal and library refurbishment during the school term.", []string{"school", "library", "classroom", "term"}, []string{"Library layout", "Classroom lighting", "Holiday works programme", "Acoustic ceiling", "ICT cable routes", "Furniture samples", "Playground access", "Safety inspection", "Defects walk-through"}, []person{{"Grace Maseko", "grace.maseko@cedarridge.example"}, {"Ethan Moore", "ethan.moore@campusworks.example"}, {"Zoe Adams", "zoe.adams@studio.example"}}},
}

var replies = []string{
	"Please review the attached proposal and confirm whether the current scope is still correct.",
	"Thanks for the update. We can proceed, provided the revised drawing is issued before Friday.",
	"The site team found a clash during the walk-through. Could you confirm the preferred route?",
	"I have checked this with our engineer. The dimensions in revision C are acceptable.",
	"Can we agree an owner for the remaining action and a date for closing it out?",
	"The supplier has confirmed availability. Delivery can be booked once the purchase order is released.",
	"Please see the revised sequence below. The change keeps the occupied area accessible.",
	"Agreed. I will update the tracker and circulate the final document tomorrow.",
}

func main() {
	var email, databaseURL, engine string
	var seed int64
	flag.StringVar(&email, "email", "", "email of an existing Automata user")
	flag.StringVar(&databaseURL, "database-url", os.Getenv("DATABASE_URL"), "local database URL/path")
	flag.StringVar(&engine, "engine", envOr("DATABASE_ENGINE", "sqlite"), "sqlite or postgres")
	flag.Int64Var(&seed, "seed", 42, "repeatable random seed")
	flag.Parse()
	if err := run(context.Background(), strings.TrimSpace(email), strings.TrimSpace(databaseURL), engine, seed); err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, email, databaseURL, engine string, seed int64) error {
	if email == "" || databaseURL == "" {
		return fmt.Errorf("-email and -database-url (or DATABASE_URL) are required")
	}
	parsedEngine, err := factory.ParseEngine(engine)
	if err != nil {
		return err
	}
	if parsedEngine != factory.EngineSQLite && parsedEngine != factory.EnginePostgres {
		return fmt.Errorf("seeddev only supports local sqlite or postgres databases")
	}
	if parsedEngine == factory.EnginePostgres {
		u, err := url.Parse(databaseURL)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") {
			return fmt.Errorf("postgres requires a local postgres:// or postgresql:// URL")
		}
		host := u.Hostname()
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("seeddev only accepts a loopback postgres host")
		}
	}
	db, err := factory.Open(ctx, factory.Config{Engine: parsedEngine, DatabaseURL: databaseURL, EnableForeignKey: true})
	if err != nil {
		return err
	}
	defer db.Close()
	var repo repository
	if parsedEngine == factory.EngineSQLite {
		repo = sqlite.NewRepository(db, 0)
	} else {
		repo = postgres.NewRepository(db, 0)
	}
	userID, _, err := repo.GetUserByEmail(ctx, email)
	if err != nil {
		return fmt.Errorf("find user %q (register in the app first): %w", email, err)
	}
	orgID, err := repo.GetHomeOrganisationID(ctx, userID)
	if err != nil {
		return fmt.Errorf("find home organisation: %w", err)
	}
	key := fmt.Sprintf("automata-seeddev:%s:%d", userID, seed)
	id := func(s string) uuid.UUID { return uuid.NewSHA1(uuid.NameSpaceOID, []byte(key+":"+s)) }
	accountID := id("account")
	account, _, err := repo.GetAccount(ctx, userID, accountID)
	if err != nil {
		return err
	}
	if account == nil {
		note := "Generated sample mailbox; no live connection"
		if err := repo.InsertAccount(ctx, domainaccounts.AccountRow{ID: accountID, UserID: userID, Label: fmt.Sprintf("Demo mail (seed %d)", seed), Provider: "m365", MsAccountKind: accounts.KindWork, PrimaryEmail: "demo@automata.invalid", ConnectionStatus: "expired", LastError: &note}, nil); err != nil {
			return fmt.Errorf("create sample mailbox: %w", err)
		}
	}
	rng := rand.New(rand.NewSource(seed))
	now := time.Now().UTC()
	lengths := []int{1, 2, 3, 5, 8, 4, 6, 7, 4} // 40 messages per project, 45 threads overall.
	for pi, sc := range scenarios {
		code := fmt.Sprintf("DEMO%d-%02d", seed, pi+1)
		projectID := id(fmt.Sprintf("project:%d", pi))
		project, err := repo.GetProjectByCode(ctx, orgID, code)
		if err != nil {
			return err
		}
		if project == nil {
			if err := repo.CreateProject(ctx, domainprojects.ProjectRow{ID: projectID, OrganisationID: orgID, Name: sc.name, Code: code, Description: &sc.description, Client: &sc.client, Keywords: sc.keywords, CreatedAt: now, UpdatedAt: now}, domainprojects.ProjectMemberRow{ID: id(fmt.Sprintf("member:%d", pi)), ProjectID: projectID, UserID: userID, Role: "Project lead", CreatedAt: now, UpdatedAt: now}); err != nil {
				return fmt.Errorf("create project %s: %w", code, err)
			}
		} else if project.ID != projectID {
			return fmt.Errorf("project code %s already belongs to another project", code)
		}
		member, err := repo.GetProjectMember(ctx, projectID, userID)
		if err != nil {
			return err
		}
		if member == nil {
			return fmt.Errorf("project %s is missing the seed user's membership", code)
		}
		for ti, count := range lengths {
			conversation := fmt.Sprintf("demo:%s:%02d", key, pi*len(lengths)+ti)
			for mi := 0; mi < count; mi++ {
				sender := sc.people[(ti+mi+rng.Intn(len(sc.people)))%len(sc.people)]
				recipient := sc.people[(ti+mi+1)%len(sc.people)]
				if recipient.address == sender.address {
					recipient = sc.people[(ti+mi+2)%len(sc.people)]
				}
				fromJSON, _ := json.Marshal(map[string]string{"name": sender.name, "address": sender.address})
				toJSON, _ := json.Marshal([]map[string]string{{"name": recipient.name, "address": recipient.address}})
				body := fmt.Sprintf("Hi %s,\n\nRegarding %s on the %s project: %s\n\nPlease let me know if anything has changed.\n\nRegards,\n%s", strings.Split(recipient.name, " ")[0], sc.topics[ti], sc.name, replies[(ti+mi+rng.Intn(len(replies)))%len(replies)], sender.name)
				subject := sc.topics[ti]
				if mi > 0 {
					subject = "Re: " + subject
				}
				received := now.Add(-time.Duration(10+pi*9+ti*3) * 24 * time.Hour).Add(time.Duration(mi*8+rng.Intn(6)) * time.Hour)
				messageKey := fmt.Sprintf("message:%d:%d:%d", pi, ti, mi)
				if err := repo.UpsertMessage(ctx, domainmessages.MessageRow{ID: id(messageKey), AccountID: accountID, ProviderMessageID: "demo:" + id(messageKey).String(), ConversationID: &conversation, ReceivedAt: received, Subject: subject, FromJSON: string(fromJSON), ToJSON: string(toJSON), CcJSON: "[]", BodyText: &body, BodyFetchedAt: &received, CreatedAt: received, UpdatedAt: received}); err != nil {
					return fmt.Errorf("write %s: %w", messageKey, err)
				}
			}
			if err := repo.UpsertThreadAssignment(ctx, domainprojects.AssignmentRow{ID: id(fmt.Sprintf("assignment:%d:%d", pi, ti)), OrganisationID: orgID, AccountID: accountID, ConversationID: conversation, ProjectID: &projectID, Status: "committed", Reason: "Generated development sample", Source: "user", AssignedByUserID: &userID, CreatedAt: now, UpdatedAt: now}); err != nil {
				return fmt.Errorf("assign %s: %w", conversation, err)
			}
		}
	}
	fmt.Printf("Seeded 5 projects, 45 threads and 200 messages for %s (seed %d).\n", email, seed)
	return nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
