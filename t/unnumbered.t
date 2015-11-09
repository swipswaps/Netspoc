#!/usr/bin/perl

use strict;
use warnings;
use Test::More;
use Test::Differences;
use lib 't';
use Test_Netspoc;

my ($title, $in, $out);

############################################################
$title = 'Zone cluster with unnumbered network';
############################################################

$in = <<'END';
network:servers = { ip = 10.1.7.32/27; }

router:r = {
 managed;
 model = IOS, FW;
 interface:servers = { ip = 10.1.7.33; hardware = e0; } 
 interface:clients = { ip = 10.1.2.1; hardware = eth1; }
 interface:unn = { unnumbered; hardware = eth2; }
}

network:unn = { unnumbered; }

router:s = {
 interface:unn;
 interface:clients = { ip = 10.1.2.2; }
}

network:clients = { ip = 10.1.2.0/24; }

pathrestriction:clients = interface:s.clients, interface:r.clients;

service:test = {
 user = any:[network:clients];
 permit src = user; dst = network:servers;
 prt = tcp 80;
}
END

$out = <<'END';
--r
ip access-list extended eth2_in
 deny ip any host 10.1.7.33
 permit tcp any 10.1.7.32 0.0.0.31 eq 80
 deny ip any any
END

test_run($title, $in, $out);


$in =~ s/\[network:clients\]/[network:unn]/msx;

test_run($title, $in, $out);

############################################################
$title = 'Auto aggregate in zone cluster with unnumbered';
############################################################

$in = <<'END';
router:Z = {
 interface:c = { unnumbered; }
 interface:L = { ip = 10.1.1.4; }
}
router:L = {
 managed;
 model = IOS;
 interface:c = { unnumbered; hardware = G2; }
 interface:L = { ip = 10.1.1.3; hardware = G0; }
}

network:c = {unnumbered;}
network:L = {ip = 10.1.1.0/24;}

pathrestriction:x = interface:Z.L, interface:L.L;

service:Test = {
 user = interface:L.[all];
 permit src = any:[user];
        dst = user;
        prt = icmp 8;
}
END

$out = <<'END';
--L
ip access-list extended G2_in
 permit icmp any host 10.1.1.3 8
 deny ip any any
--
ip access-list extended G0_in
 permit icmp any host 10.1.1.3 8
 deny ip any any
END

test_run($title, $in, $out);


$in =~ s|\[user\]|[ip=10.0.0.0/8 & user]|;

$out = <<'END';
--L
ip access-list extended G2_in
 permit icmp 10.0.0.0 0.255.255.255 host 10.1.1.3 8
 deny ip any any
--
ip access-list extended G0_in
 permit icmp 10.0.0.0 0.255.255.255 host 10.1.1.3 8
 deny ip any any
END

test_run($title, $in, $out);

############################################################
$title = 'Auto interface expands to short interface';
############################################################

$in = <<'END';
router:u1 = {
 model = IOS;
 interface:dummy;
}

network:dummy = { unnumbered; }

router:u2 = {
 interface:dummy = { unnumbered; }
 interface:n1 = { ip = 10.1.1.2; }
}

network:n1 = { ip = 10.1.1.0/24; }

router:r1 = {
 managed;
 model = ASA;
 interface:n1 = {ip = 10.1.1.1; hardware = n1; }
 interface:n2 = {ip = 10.1.2.1; hardware = n2; }
}

network:n2 = { ip = 10.1.2.0/24; }

service:s1 = {
 user = interface:u1.[auto];
 permit src = network:n2;
        dst = user;
	prt = tcp 22;
}
END

$out = <<'END';
Error: 'short' interface:u1.dummy (from .[auto])
 must not be used in rule of service:s1
END

test_err($title, $in, $out);

############################################################
$title = 'Auto interface expands to unnumbered interface';
############################################################
# and this unnumbered interface is silently ignored.

$in =~ s/interface:dummy;/interface:dummy = { unnumbered; }/;

$out = <<'END';
--r1
! [ ACL ]
access-list n1_in extended deny ip any any
access-group n1_in in interface n1
END

test_run($title, $in, $out);

############################################################

done_testing;
